package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/shell"
	"github.com/kl09/mac-pulse/internal/store"
	"github.com/kl09/mac-pulse/internal/ui"
)

// session joins the collectors, the stores and the shell: samples flow out to the menu
// bar and the web views, frontend messages flow in.
// open(1) answers in tens of milliseconds; this only bounds a hung LaunchServices.
const openTimeout = 5 * time.Second

type session struct {
	quit      func()
	self      ui.Self
	settings  *settings.Store
	history   *store.Store
	alerts    *alerts.Engine
	latest    *latest
	sampler   *collector.Sampler
	inspector *netinspect.Inspector
	// devInspector is the slow poll behind the Dev tab's listeners, run while no network screen needs the fast one.
	devInspector *netinspect.Inspector
	docker       *collector.Docker
	describer    collector.Describer
	// home is "" when the home directory is unknown: no dev path is shortened then.
	home string
	// systemLang is what the "system" language setting resolves to; macOS fixes it at launch.
	systemLang string
	// statusFixed draws the status item from fixedSample: the MAC_PULSE_STATUS_FIXED debug hook.
	statusFixed bool

	// Everything below belongs to the shell's message goroutine, and to main once
	// shell.Run has returned. tabs and visible are keyed by view mode.
	tabs    map[string]string
	visible map[string]bool
	// endNet and endDocker are nil while the inspectors and the docker loop are stopped;
	// netFast says which inspector endNet stops.
	endNet    func()
	netFast   bool
	endDocker func()
	// askPending is set by the quit-heaviest shortcut: the panel's next tab message gets window.mp.ask().
	askPending bool
	// anyVisible and detail are what the sampler was last told; SetInterval and SetDetail
	// reset its timer, so they must not be called for every message.
	anyVisible bool
	detail     collector.Detail

	// lookups counts the goroutines of the slow actions; main waits for it after shell.Run.
	lookups sync.WaitGroup
	// One request of a kind at a time: a second click while the first is out is dropped.
	ipBusy    atomic.Bool
	pingBusy  atomic.Bool
	speedBusy atomic.Bool
	ejectBusy atomic.Bool
	tabsBusy  atomic.Bool
	// detailBusy also bounds the lsof processes a burst of proc_detail messages could start.
	detailBusy atomic.Bool
	updateBusy atomic.Bool
	scanBusy   atomic.Bool
	// cleanBusy covers the measurement and the cleanup alike: a cleanup removes what was measured.
	cleanBusy atomic.Bool
	// noTrash holds the categories whose last cleanup found no Trash: the one thing that lets
	// the next cleanup of such a category delete for good. Only the goroutine that holds
	// cleanBusy reads and writes it.
	noTrash map[string]bool
	// cancelScan stops the storage scan that runs; calling it after the scan has ended does
	// nothing. It belongs to the message goroutine.
	cancelScan context.CancelFunc
	// statusMu makes one drawStatus a single step: samples, network reports and messages
	// all redraw the menu bar from goroutines of their own.
	statusMu sync.Mutex
	// separate is true while the separate status items are in the menu bar; statusMu guards it.
	separate bool
}

// onSample runs on the sampler goroutine.
func (s *session) onSample(snap *collector.Snapshot) {
	s.history.Add(snap)
	s.alerts.Check(snap, s.settings.Get())
	s.latest.update(func(c *current) { c.snap = snap })
	s.refresh()
}

// group names the app of a pid the way the last sample's Apps list groups it, so that the
// network collectors put a process's traffic under the name its CPU and memory have. It
// runs on their goroutines.
// A linear search per pid, a few hundred pids a poll; index the sample by pid if
// a poll ever shows in a profile.
func (s *session) group(pid int32) (name, bundlePath string, ok bool) {
	snap := s.latest.get().snap
	if snap == nil {
		return "", "", false
	}
	i := slices.IndexFunc(snap.Apps, func(a collector.App) bool { return slices.Contains(a.PIDs, pid) })
	if i < 0 {
		return "", "", false
	}
	return snap.Apps[i].Name, snap.Apps[i].BundlePath, true
}

// onNet runs on the inspector goroutine.
func (s *session) onNet(rep *netinspect.Report) {
	s.latest.update(func(c *current) { c.net = rep })
	s.refresh()
}

// onDocker runs on the docker loop's goroutine.
func (s *session) onDocker(rep *collector.DockerReport) {
	s.latest.update(func(c *current) { c.docker = rep })
	s.refresh()
}

// refresh redraws the menu bar and, while a view is on screen, pushes a new state; a
// hidden view gets none, and only the Apps tab gets the full app list (~150 KB).
func (s *session) refresh() {
	cur := s.latest.get()
	if cur.snap == nil {
		return
	}
	if s.statusFixed {
		s.drawStatus(&fixedSample, false)
	} else {
		s.drawStatus(cur.snap, len(s.alerts.Active()) > 0)
	}
	if !cur.visible {
		return
	}
	b, err := encodeState(s.state(cur))
	if err != nil {
		slog.Error("marshal state", "err", err)
		return
	}
	shell.Push("onState", b)
}

// encodeState is the state as the page gets it. The params of an alert are the one part of
// it that is free-form: when they hold what JSON cannot say, the state goes out without the
// alerts instead of not at all, which would leave every screen empty.
func encodeState(st ui.State) ([]byte, error) {
	b, err := json.Marshal(st)
	if err == nil {
		return b, nil
	}
	slog.Error("marshal state: sent without its alerts", "err", err)
	st.Alerts = ui.Alerts{Active: []ui.Alert{}, Recent: []ui.Alert{}}
	return json.Marshal(st)
}

// drawStatus hands the shell the menu bar title: one status item, or with the
// menu_bar_separate setting one item per entry, each opening its own screen.
func (s *session) drawStatus(snap *collector.Snapshot, alert bool) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	// Read under the lock: a draw that waited must not put back the mode of an older setting.
	cfg, spark := s.settings.Get(), s.history.Spark()
	was := s.separate
	s.separate = cfg.MenuBarSeparate
	if !cfg.MenuBarSeparate {
		shell.SetStatus(statusItems(snap, spark, cfg, alert))
		if !was {
			return
		}
	} else {
		shell.SetStatus(nil)
	}
	groups := statusGroups(snap, spark, cfg, alert)
	for _, key := range statusOrder(cfg.MenuBar) {
		// nil takes away the item of an entry that left the menu bar, or of the mode switched off.
		var items []shell.StatusItem
		if i := slices.Index(cfg.MenuBar, key); i >= 0 && cfg.MenuBarSeparate {
			items = groups[i]
		}
		shell.SetExtraStatus(key, statusTabs[key], items)
	}
}

// statusOrder is the order the separate status items go to the shell in: the entries of the
// menu_bar setting as the Settings list has them, so that items without a saved place appear
// in that order, then the keys that are not in the menu bar, whose items are only taken away.
func statusOrder(menuBar []string) []string {
	order := slices.Clone(menuBar)
	for _, key := range statusKeys {
		if !slices.Contains(order, key) {
			order = append(order, key)
		}
	}
	return order
}

func (s *session) state(cur current) ui.State {
	in := ui.Input{
		Snap: cur.snap, Spark: s.history.Spark(), Today: s.history.Today(),
		Active: s.alerts.Active(), Recent: s.alerts.Recent(),
		Settings: s.settings.Get(), SystemLang: s.systemLang, LoginItem: shell.LoginItem(), Self: s.self,
		AllApps: cur.allApps, Pinned: cur.pinned, Docker: cur.docker, Home: s.home,
	}
	if cur.sendNet {
		in.Net = cur.net
	}
	if cur.net != nil {
		in.Listening = cur.net.Listening
	}
	if cur.snap.NetInfo != nil {
		in.NetTotals = s.history.NetTotals()
	}
	if cur.storageTab {
		in.Storage = &cur.storage
	}
	return ui.Build(in)
}

// lang is the language of the texts Go renders itself: notifications and the status item's menu.
func (s *session) lang() string {
	if l := s.settings.Get().Language; l != settings.LanguageSystem {
		return l
	}
	return s.systemLang
}

func (s *session) setMenu() {
	l := s.lang()
	shell.SetMenu(ui.Translate(l, "menu.open", nil), ui.Translate(l, "open_window", nil),
		ui.Translate(l, "menu.export", nil), ui.Translate(l, "menu.settings", nil), ui.Translate(l, "set.quit", nil),
		ui.Translate(l, "menu.reset_panel", nil))
}

// notifyAlert runs on the sampler goroutine.
func (s *session) notifyAlert(a alerts.Alert) {
	title, body := ui.AlertTexts(s.lang(), a)
	slog.Info("alert", "title", title, "body", body)
	// A click on the banner opens the panel on this alert.
	shell.Notify(title, body, ui.TabAlert+":"+a.ID)
}

// flush writes the history, and with it the alerts a restart brings back.
func (s *session) flush() error {
	s.history.SetAlerts(s.alerts.Closed())
	return s.history.Flush()
}

// answer reports the outcome of a button action: key names a dictionary string, text is
// its {text} or, without a key, the whole answer.
func (s *session) answer(action string, ok bool, key, text string) {
	s.push(ui.Action{Action: action, OK: ok, Key: key, Text: text})
}

func (s *session) push(a ui.Action) {
	slog.Debug("action", "action", a.Action, "ok", a.OK, "key", a.Key, "text", a.Text, "values", a.Values, "info", a.Info,
		"list", len(a.List), "detail", a.Detail != nil, "review", a.Review != nil, "alert", a.Alert != nil)
	// Strings, a bool and finite numbers always marshal.
	b, _ := json.Marshal(a)
	shell.Push("onAction", b)
}

// fail answers with the system's own words for err, which have no translation.
func (s *session) fail(action string, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		s.answer(action, false, "err.timeout", "")
		return
	}
	// The request's URL and the wrapped chain mean nothing in a one-line row.
	if urlErr := (*url.Error)(nil); errors.As(err, &urlErr) {
		err = urlErr.Unwrap()
	}
	s.answer(action, false, "", err.Error())
}

func (s *session) notice(level string, texts ...ui.Text) {
	b, err := json.Marshal(ui.Notice{Level: level, Texts: texts})
	if err != nil {
		slog.Error("marshal notice", "err", err)
		return
	}
	shell.Push("onNotice", b)
}

func (s *session) onMessage(ctx context.Context, raw []byte) {
	// Once shutdown has begun the main run loop may be gone, and a shell call that waits for
	// it would hang the exit before the final history flush.
	if ctx.Err() != nil {
		return
	}
	slog.Debug("message", "raw", string(raw))
	msg, err := ui.ParseMessage(raw)
	if err != nil {
		slog.Warn("message dropped", "err", err)
		return
	}
	if s.command(ctx, msg) {
		return
	}
	switch msg.Type {
	case "set":
		s.set(msg)
	case "login_item":
		if err := shell.SetLoginItem(msg.Enabled); err != nil {
			slog.Warn("set login item", "err", err)
			s.notice("error", ui.Text{Key: "notice.login_item", Params: map[string]any{"reason": err.Error()}})
		}
	case "tab":
		s.tabs[msg.Mode] = msg.Name
		s.syncViews(ctx)
	case "visibility":
		s.setVisibility(ctx, msg)
	}
	// A view that just appeared, switched tabs or changed a setting must not wait for the next tick.
	s.refresh()
	// After the state, which the page picks the heaviest app from.
	// An open window gets the confirm too; Push has no single-view form.
	if s.askPending && msg.Type == "tab" && msg.Mode == "popover" {
		s.askPending = false
		shell.Push("ask", nil)
	}
}

// command runs a message that changes nothing a view shows by itself, and says whether
// msg was one; the rest are left to onMessage, which pushes a fresh state after them.
func (s *session) command(ctx context.Context, msg ui.Message) bool {
	switch msg.Type {
	case "quit":
		s.quit()
	case "open_window":
		shell.OpenWindow()
	case "open_panel":
		shell.BackToPanel()
	case "log":
		slog.Warn("frontend", "message", msg.Message)
	case "history":
		s.pushHistory(msg)
	case "quit_app":
		s.quitApp(ctx, msg)
	case "public_ip", "ping", "export", "copy", "speedtest", "export_csv", "update_check":
		s.act(ctx, msg.Type)
	case "storage_scan", "storage_cancel", "storage_clear", "storage_open", "cleanup_scan", "cleanup_list", "cleanup":
		s.storageCommand(ctx, msg)
	case "pin":
		// Only the panel has a pin, and only while it is up; the shell answers with a visibility message.
		if s.visible["popover"] {
			shell.SetPinned(msg.Enabled)
		}
	case "url":
		shell.ShowPanel(msg.Name)
	case "hotkey":
		s.askPending = true
		shell.ShowPanel(ui.TabApps)
	default:
		return s.target(ctx, msg)
	}
	return true
}

// target is command for the messages that pick one volume, process, app or address, split for gocyclo.
func (s *session) target(ctx context.Context, msg ui.Message) bool {
	switch msg.Type {
	case "eject":
		s.eject(ctx, msg.Mount)
	case "reveal":
		s.reveal(ctx, msg)
	case "app_info":
		s.appInfo(ctx, msg)
	case "proc_detail":
		s.procDetail(ctx, msg)
	case "alert_detail":
		rec, ok := s.alerts.Record(msg.ID)
		if !ok {
			// The list it was picked from is older than the 20 alerts kept, or the id is nobody's.
			s.push(ui.Action{Action: msg.Type, Key: "alertd.gone", Text: msg.ID})
			break
		}
		s.push(ui.Action{Action: msg.Type, OK: true, Alert: ui.BuildAlertDetail(rec)})
	case "browser_tabs":
		s.browserTabs(ctx, msg)
	case "open":
		// The address is Go's own: the message only picked its key.
		if out, err := openWith(ctx, ui.OpenURLs[msg.Target]); err != nil {
			slog.Warn("open", "target", msg.Target, "err", err, "output", string(out))
			s.fail(msg.Type, err)
		}
	default:
		return false
	}
	return true
}

func (s *session) setVisibility(ctx context.Context, msg ui.Message) {
	s.visible["popover"], s.visible["window"] = msg.Popover, msg.Window
	visible := msg.Popover || msg.Window
	if !msg.Popover {
		// A panel that closed before it reported its tab must not show the confirm the next time it opens.
		s.askPending = false
	}
	// The shell clears the pin whenever the panel closes.
	s.latest.update(func(c *current) { c.visible, c.pinned = visible, msg.Pinned && msg.Popover })
	if visible != s.anyVisible {
		s.anyVisible = visible
		interval := hiddenInterval
		if visible {
			interval = visibleInterval
		}
		s.sampler.SetInterval(interval)
	}
	s.syncViews(ctx)
}

// set stores one setting. The caller pushes the state back even after a refusal, so the
// control snaps back.
func (s *session) set(msg ui.Message) {
	if err := s.settings.Set(msg.Key, msg.Value); err != nil {
		slog.Warn("set setting", "err", err)
		s.notice("error", ui.Text{Key: "notice.setting", Params: map[string]any{"reason": err.Error()}})
		return
	}
	switch msg.Key {
	case "language":
		s.setMenu()
		// The clock names the day and the month in the interface language.
		s.applyLook()
	case "hotkey", "hotkey_quit":
		if err := s.applyHotkey(msg.Key); err != nil {
			slog.Warn("set hotkey", "err", err)
			s.notice("error", ui.Text{Key: "notice.hotkey"})
		}
	case "window_on_top", "menu_bar_compact", "appearance", "theme", "show_in_dock", "clock", "clock_date", "clock_seconds", "clock_hours":
		s.applyLook()
	}
}

// applySettings hands the shell, before it runs, everything the settings ask of it.
func (s *session) applySettings() {
	s.setMenu()
	s.applyLook()
	cfg := s.settings.Get()
	for key, on := range map[string]bool{"hotkey": cfg.Hotkey, "hotkey_quit": cfg.HotkeyQuit} {
		if !on {
			continue
		}
		// No view is up yet to show a notice; the setting is already back to off.
		if err := s.applyHotkey(key); err != nil {
			slog.Warn("set hotkey", "err", err)
		}
	}
}

// applyLook hands the shell the settings it draws by itself.
func (s *session) applyLook() {
	cfg := s.settings.Get()
	shell.SetWindowOnTop(cfg.WindowOnTop)
	shell.SetStatusStyle(cfg.MenuBarCompact)
	shell.SetDock(cfg.ShowInDock)
	shell.SetClock(clockTemplate(cfg), s.lang())
	// A one-mode theme outranks the setting, as on the page: the native parts must match it.
	mode := cfg.Appearance
	if only, ok := settings.ThemeMode[cfg.Theme]; ok {
		mode = only
	}
	shell.SetAppearance(mode)
}

// act runs one of the button actions that carry no fields and answers through onAction.
func (s *session) act(ctx context.Context, action string) {
	switch action {
	case "public_ip":
		s.lookup(ctx, action, &s.ipBusy, func(ctx context.Context) (ui.Action, error) {
			addr, err := netinspect.PublicIP(ctx)
			return ui.Action{Text: addr.String()}, err
		})
	case "ping":
		s.lookup(ctx, action, &s.pingBusy, func(ctx context.Context) (ui.Action, error) {
			rtt, err := netinspect.Ping(ctx)
			return ui.Action{Text: fmt.Sprintf("%.0f ms", float64(rtt)/float64(time.Millisecond))}, err
		})
	case "speedtest":
		s.lookup(ctx, action, &s.speedBusy, func(ctx context.Context) (ui.Action, error) {
			speed, err := netinspect.SpeedTest(ctx)
			return ui.Action{Values: map[string]float64{"down": speed.Down, "up": speed.Up, "rpm": speed.RPM, "rtt_ms": speed.RTTms}}, err
		})
	case "update_check":
		s.lookup(ctx, action, &s.updateBusy, func(ctx context.Context) (ui.Action, error) {
			tag, err := netinspect.LatestRelease(ctx)
			switch {
			case errors.Is(err, netinspect.ErrNoRelease):
				return ui.Action{Key: "upd.none"}, nil
			case err != nil:
				return ui.Action{Key: "err.update"}, err
			case netinspect.Newer(tag, version):
				return ui.Action{Key: "upd.newer", Text: tag}, nil
			}
			return ui.Action{Key: "upd.latest"}, nil
		})
	case "export":
		s.export()
	case "export_csv":
		s.exportCSV()
	case "copy":
		cur := s.latest.get()
		if cur.snap == nil {
			s.answer(action, false, "err.no_sample", "")
			return
		}
		cur.allApps = false
		shell.Copy(ui.Summary(s.state(cur)))
		s.answer(action, true, "act.copied", "")
	}
}

// lookup runs a slow action off the message goroutine: an internet request or an eject
// may take seconds, and the panel must keep answering meanwhile. A failure whose answer
// has a Key is told in the dictionary's words, any other in the system's own. started is
// false when the slot is busy and run was dropped: the caller answers that where a page
// waits for an answer of its own.
func (s *session) lookup(
	ctx context.Context, action string, busy *atomic.Bool, run func(context.Context) (ui.Action, error),
) (started bool) {
	if !busy.CompareAndSwap(false, true) {
		return false
	}
	s.lookups.Go(func() {
		defer busy.Store(false)
		res, err := run(ctx)
		// During shutdown the web views are already gone.
		switch {
		case ctx.Err() != nil:
		case err != nil && res.Key == "":
			s.fail(action, err)
		default:
			if err != nil {
				slog.Warn("action failed", "action", action, "err", err)
			}
			res.Action, res.OK = action, err == nil
			s.push(res)
		}
	})
	return true
}

// eject unmounts a volume the last sample listed as ejectable; the web view's path is only
// ever compared with those.
func (s *session) eject(ctx context.Context, mount string) {
	const action = "eject"
	var volume collector.Volume
	if snap := s.latest.get().snap; snap != nil {
		volume = ui.EjectVolume(snap.Disk.Volumes, mount)
	}
	if volume.Mount == "" {
		// The volume left between the sample and the click.
		slog.Warn("eject refused: not an ejectable volume of the last sample", "mount", mount)
		s.answer(action, false, "err.eject_gone", mount)
		return
	}
	s.lookup(ctx, action, &s.ejectBusy, func(ctx context.Context) (ui.Action, error) {
		return ui.Action{Key: "act.ejected", Text: collector.CleanText(volume.Name, 0)}, collector.Eject(ctx, volume.Mount)
	})
}

// reveal shows a volume, a process's bundle or a scanned folder in Finder. The path is Go's
// own: the message only picks a mount or a pid of the last sample, or a node of the scanned tree.
func (s *session) reveal(ctx context.Context, msg ui.Message) {
	var args []string
	switch cur := s.latest.get(); {
	case msg.Path != "":
		if path := ui.ScannedPath(cur.tree, cur.storage.Scan.Root, msg.Path); path != "" {
			args = []string{"-R", path}
		}
	case cur.snap == nil:
	case msg.Mount == "":
		if path := ui.RevealPath(cur.snap.Apps, msg.PID, s.self); path != "" {
			args = []string{"-R", path}
		}
	default:
		// The page sends the mount it was shown, which has the bidi controls removed.
		volumes := cur.snap.Disk.Volumes
		if i := slices.IndexFunc(volumes, func(v collector.Volume) bool { return collector.CleanText(v.Mount, 0) == msg.Mount }); i >= 0 {
			args = []string{volumes[i].Mount}
		}
	}
	if args == nil {
		slog.Warn("reveal refused: not a volume, an own process or a scanned path", "mount", msg.Mount, "pid", msg.PID, "path", msg.Path)
		s.notice("error", ui.Text{Key: "err.reveal"})
		return
	}
	if out, err := openWith(ctx, args...); err != nil {
		slog.Warn("reveal in Finder", "err", err, "output", string(out))
		s.notice("error", ui.Text{Key: "err.reveal"})
	}
}

// openWith runs open(1): args are a path to show in Finder or an address of ui.OpenURLs.
func openWith(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "open", args...).CombinedOutput()
}

// appInfo answers what an app or a process of the last sample is. Reading its bundle,
// signature and man page takes up to seconds, so that runs off the message goroutine.
func (s *session) appInfo(ctx context.Context, msg ui.Message) {
	const action = "app_info"
	var apps []collector.App
	if snap := s.latest.get().snap; snap != nil {
		apps = snap.Apps
	}
	info, target, path, ok := ui.InfoTarget(apps, msg, s.self)
	if !ok {
		s.push(ui.Action{Action: action, Key: "info.gone", Info: &ui.AppInfo{Name: msg.Name, PID: msg.PID}})
		return
	}
	s.lookups.Go(func() {
		manual := ""
		if info.DescKey == "" {
			manual = target.Name
		}
		d := s.describer.Describe(ctx, path, target.Exe, manual)
		// The title stays the name of the folder on disk; what the bundle calls itself is its own claim.
		info.BundleName, info.Version, info.BundleID = d.Name, d.Version, d.BundleID
		info.Developer, info.Category, info.Manual = d.Copyright, d.Category, d.Manual
		info.Signing, info.Signer, info.ExecutableOnly = cmp.Or(d.Signing, info.Signing), d.Signer, d.ExecutableOnly
		if at := collector.StartedAt(ctx, target.PID); !at.IsZero() {
			info.Since = at.UnixMilli()
		}
		if owner, err := user.LookupId(strconv.FormatUint(uint64(target.UID), 10)); err == nil {
			info.User = owner.Username
		}
		// During shutdown the web views are already gone.
		if ctx.Err() == nil {
			s.push(ui.Action{Action: action, OK: true, Info: &info})
		}
	})
}

// procDetail answers what a process of the user's own has open. The message only picks a
// process of the last sample; its parents come from that sample too.
func (s *session) procDetail(ctx context.Context, msg ui.Message) {
	const action = "proc_detail"
	var apps []collector.App
	if snap := s.latest.get().snap; snap != nil {
		apps = snap.Apps
	}
	target, ok := ui.DetailTarget(apps, msg, s.self)
	if !ok {
		s.answer(action, false, "info.gone", "")
		return
	}
	s.lookup(ctx, action, &s.detailBusy, func(ctx context.Context) (ui.Action, error) {
		d, err := collector.ReadProcDetail(ctx, target.PID, msg.Args)
		if err != nil {
			return ui.Action{Key: "info.gone"}, err
		}
		return ui.Action{Detail: ui.BuildProcDetail(apps, target, d)}, nil
	})
}

// browserTabs reads the tab titles of a browser of the last sample: one that is not
// running would be launched by the script.
func (s *session) browserTabs(ctx context.Context, msg ui.Message) {
	const action = "browser_tabs"
	snap := s.latest.get().snap
	if snap == nil || !ui.BrowserRunning(snap.Apps, msg.Name) {
		s.answer(action, false, "info.gone", msg.Name)
		return
	}
	started := s.lookup(ctx, action, &s.tabsBusy, func(ctx context.Context) (ui.Action, error) {
		titles, err := collector.BrowserTabs(ctx, msg.Name)
		switch {
		case errors.Is(err, collector.ErrDenied):
			return ui.Action{Key: "err.automation", Text: msg.Name}, err
		// The page finds the row by the browser's name: every failure carries it too.
		case errors.Is(err, collector.ErrNotRunning):
			return ui.Action{Key: "info.gone", Text: msg.Name}, err
		case errors.Is(err, context.DeadlineExceeded):
			return ui.Action{Key: "err.timeout", Text: msg.Name}, err
		case err != nil:
			return ui.Action{Key: "act.no_answer", Text: msg.Name}, err
		}
		return ui.Action{Text: msg.Name, List: titles}, nil
	})
	// One browser at a time: the first may be waiting for the user to answer the macOS prompt.
	if !started {
		s.answer(action, false, "err.busy", msg.Name)
	}
}

// exportCSV writes the history as two CSV files where export puts its PNG.
func (s *session) exportCSV() {
	const action = "export_csv"
	home, err := os.UserHomeDir()
	if err != nil {
		s.fail(action, fmt.Errorf("locate home directory: %w", err))
		return
	}
	name := time.Now().Format("mac-pulse-20060102-150405")
	pictures := filepath.Join("Pictures", "mac-pulse")
	for _, folder := range []string{"Desktop", pictures} {
		if folder == pictures {
			if err := os.MkdirAll(filepath.Join(home, folder), 0o755); err != nil {
				slog.Warn("export CSV", "err", err)
				break
			}
		}
		err := writeCSV(s.history, filepath.Join(home, folder, name))
		if err == nil {
			s.answer(action, true, "act.saved", filepath.Join(folder, name+"-system.csv"))
			return
		}
		slog.Warn("export CSV", "folder", folder, "err", err)
	}
	s.answer(action, false, "err.not_saved", "")
}

// writeCSV creates <base>-system.csv and <base>-apps.csv, never over an existing file, and
// leaves neither behind when one fails.
func writeCSV(history *store.Store, base string) (err error) {
	files := make([]*os.File, 0, 2)
	defer func() {
		for _, f := range files {
			err = errors.Join(err, f.Close())
		}
		if err != nil {
			for _, f := range files {
				_ = os.Remove(f.Name())
			}
		}
	}()
	for _, suffix := range []string{"-system.csv", "-apps.csv"} {
		f, err := os.OpenFile(base+suffix, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create CSV: %w", err)
		}
		files = append(files, f)
	}
	return history.WriteCSV(files[0], files[1])
}

// export saves the view on screen as a PNG. The path is chosen here, never by the web
// view: the Desktop, or ~/Pictures/mac-pulse when macOS denies the Desktop.
func (s *session) export() {
	const action = "export"
	if !s.anyVisible {
		s.answer(action, false, "err.panel_closed", "")
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		s.fail(action, fmt.Errorf("locate home directory: %w", err))
		return
	}
	name := time.Now().Format("mac-pulse-20060102-150405.png")
	folder := "Desktop"
	if err = shell.Export(filepath.Join(home, folder, name)); err != nil {
		slog.Warn("export to Desktop", "err", err)
		folder = filepath.Join("Pictures", "mac-pulse")
		if err = os.MkdirAll(filepath.Join(home, folder), 0o755); err == nil {
			err = shell.Export(filepath.Join(home, folder, name))
		}
		if err != nil {
			slog.Warn("export to Pictures", "err", err)
			s.answer(action, false, "err.not_saved", "")
			return
		}
	}
	s.answer(action, true, "act.saved", filepath.Join(folder, name))
}

// applyHotkey registers or releases the global shortcut of the setting key ("hotkey" or
// "hotkey_quit") to match it; when the registration fails the setting goes back to off.
func (s *session) applyHotkey(key string) error {
	cfg := s.settings.Get()
	id, on := shell.HotkeyPanel, cfg.Hotkey
	if key == "hotkey_quit" {
		id, on = shell.HotkeyQuit, cfg.HotkeyQuit
	}
	err := shell.SetHotkey(id, on)
	if err == nil {
		return nil
	}
	if err := s.settings.Set(key, json.RawMessage("false")); err != nil {
		slog.Warn("reset hotkey setting", "key", key, "err", err)
	}
	return fmt.Errorf("apply %s setting: %w", key, err)
}

func (s *session) pushHistory(msg ui.Message) {
	h, err := s.history.History(msg.Metric, msg.Range)
	if err != nil {
		slog.Warn("history request", "err", err)
		return
	}
	var system map[string]bool
	if snap := s.latest.get().snap; snap != nil {
		system = ui.SystemApps(snap.Apps, s.self)
	}
	b, err := json.Marshal(ui.BuildHistory(h, system))
	if err != nil {
		slog.Error("marshal history", "err", err)
		return
	}
	shell.Push("onHistory", b)
}

func (s *session) quitApp(ctx context.Context, msg ui.Message) {
	apps, err := collector.Apps(ctx)
	if err != nil {
		slog.Warn("scan processes before quit", "err", err)
		s.notice("error", ui.Text{Key: "notice.scan"})
		return
	}
	targets, problems := ui.QuitTargets(apps, msg, s.self)
	sig := syscall.SIGTERM
	if msg.Force {
		sig = syscall.SIGKILL
	}
	// A named signal goes to the process as it is, never through the app's own quit.
	if named, ok := ui.Signals[msg.Signal]; ok {
		sig = named
	}
	var asked []int32
	for _, pid := range targets {
		// An application gets the Cmd+Q treatment and may ask to save; a plain process gets the signal.
		if !msg.Force && msg.Signal == "" && shell.TerminateApp(pid) {
			asked = append(asked, pid)
			continue
		}
		err := syscall.Kill(int(pid), sig)
		switch {
		// ESRCH: it exited on its own between the scan and the signal.
		case err == nil, errors.Is(err, syscall.ESRCH):
		case errors.Is(err, syscall.EPERM):
			problems = append(problems, ui.Text{Key: "quit.not_owned_pid", Params: map[string]any{"pid": pid}})
		default:
			problems = append(problems, ui.Text{Key: "quit.failed", Params: map[string]any{"pid": pid, "reason": err.Error()}})
		}
	}
	slog.Info("quit app", "app", msg.App, "signal", sig, "pids", targets, "terminate_request", asked, "refused", len(problems))
	if len(problems) > 0 {
		s.notice("error", problems...)
		return
	}
	switch {
	case msg.Signal == "":
	case len(targets) > 0:
		s.answer("quit_app", true, "act.signal", msg.Signal)
	// The process exited between the page's sample and the scan above.
	default:
		s.answer("quit_app", false, "info.gone", "")
	}
}

// viewNeeds is what the views on screen ask for: the network report for the two network
// screens, reverse DNS for the Network tab alone (the only screen that shows hosts), every
// app for the Apps tab, the storage part for the two screens with the cleanup (the Storage
// tab and the disk detail) and the detail collectors of an open detail screen or of the Dev
// tab, whose listeners, containers and agents all hang on detail.Dev. A hidden view asks
// for nothing, whatever tab it was left on.
func viewNeeds(tabs map[string]string, visible map[string]bool) (net, resolve, allApps, storage bool, detail collector.Detail) {
	for mode, tab := range tabs {
		if !visible[mode] {
			continue
		}
		switch tab {
		case ui.TabApps:
			allApps = true
		case ui.TabNetwork:
			net, resolve = true, true
		case ui.TabNetInfo:
			net, detail.NetInfo = true, true
		case ui.TabDev:
			detail.Dev = true
		case ui.TabCPU, ui.TabGPU:
			detail.Temps = true
		case ui.TabSensors:
			detail.Temps, detail.Bluetooth = true, true
		case ui.TabDisk:
			detail.SMART, storage = true, true
		case ui.TabStorage:
			storage = true
		}
	}
	return net, resolve, allApps, storage, detail
}

// syncViews starts and stops the collectors that only an open screen needs.
func (s *session) syncViews(ctx context.Context) {
	net, resolve, allApps, storageTab, detail := viewNeeds(s.tabs, s.visible)
	s.latest.update(func(c *current) { c.allApps, c.sendNet, c.storageTab = allApps, net, storageTab })
	s.inspector.SetResolve(resolve)
	if detail != s.detail {
		s.detail = detail
		s.sampler.SetDetail(detail)
	}
	// The Dev tab reads its listeners from an inspector too; a slow one will do for it.
	if wantNet := net || detail.Dev; !wantNet || s.netFast != net {
		s.stopInspector()
	}
	switch {
	case net && s.endNet == nil:
		s.endNet, s.netFast = start(ctx, s.inspector.Run), true
	case detail.Dev && s.endNet == nil:
		s.endNet, s.netFast = start(ctx, s.devInspector.Run), false
	}
	switch {
	case detail.Dev && s.endDocker == nil:
		s.endDocker = start(ctx, s.docker.Run)
	case !detail.Dev:
		s.stopDocker()
	}
}

// start runs a collector loop until the returned stop, which waits for the loop to end.
// stop waits on the message goroutine; cancelling kills the loop's child process,
// so that is milliseconds. Hand it to a goroutine if messages ever queue up.
func start(ctx context.Context, run func(context.Context)) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

// stopInspector ends the inspector that runs and drops what it last reported.
func (s *session) stopInspector() {
	if s.endNet != nil {
		s.endNet()
		s.endNet = nil
		s.latest.update(func(c *current) { c.net = nil })
	}
}

// stopDocker ends the docker loop and drops what it last reported.
func (s *session) stopDocker() {
	if s.endDocker != nil {
		s.endDocker()
		s.endDocker = nil
		s.latest.update(func(c *current) { c.docker = nil })
	}
}
