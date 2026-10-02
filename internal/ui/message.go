package ui

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/storage"
)

const (
	TabApps    = "processes"
	TabNetwork = "network"
	// The detail screens are tabs too: a tile on the overview pushes one.
	TabCPU     = "detail:cpu"
	TabGPU     = "detail:gpu"
	TabMemory  = "detail:memory"
	TabDisk    = "detail:disk"
	TabNetInfo = "detail:network"
	TabBattery = "detail:battery"
	TabSensors = "detail:sensors"
	TabDev     = "dev"
	TabStorage = "storage"
	TabClock   = "detail:clock"
	// TabAlert shows one alert; the page asks for it with an alert_detail message.
	TabAlert = "detail:alert"
	// A real app has tens of processes; a list this long did not come from the frontend.
	maxQuitPIDs = 1024
	maxLogBytes = 2048
	// A mount point is a path, and PATH_MAX is 1024.
	maxMountBytes = 1024
	// An app is named after a file, and NAME_MAX is 255.
	maxNameBytes = 255
	// A real parent chain is under ten processes long.
	maxChain = 64
	// An alert id is a kind, a name of at most NAME_MAX bytes and a time: a longer one is cut.
	maxAlertID = 320
)

var ErrBadMessage = errors.New("bad message")

// Signals are what a quit_app message may send in place of a quit, by the name it carries.
var Signals = map[string]syscall.Signal{
	"TERM": syscall.SIGTERM, "KILL": syscall.SIGKILL, "HUP": syscall.SIGHUP,
	"INT": syscall.SIGINT, "STOP": syscall.SIGSTOP, "CONT": syscall.SIGCONT,
}

// OpenURLs are what an open message may hand to open(1), by target: a pane of System
// Settings or the releases page. The page picks a key; the address is never the page's.
var OpenURLs = map[string]string{
	"battery":            "x-apple.systempreferences:com.apple.Battery-Settings.extension",
	"privacy_files":      "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_FilesAndFolders",
	"privacy_full_disk":  "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_AllFiles",
	"privacy_automation": "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_Automation",
	"releases":           "https://github.com/kl09/mac-pulse/releases/latest",
}

// tabs are the names a view may be on: what a tab message reports and a mac-pulse:// link asks for.
var tabs = []string{
	"overview", TabApps, TabNetwork, "history", "settings",
	TabCPU, TabGPU, TabMemory, TabDisk, TabNetInfo, TabBattery, TabSensors, TabDev, TabStorage, TabClock, TabAlert,
	// The sections of the settings, SECTIONS in app.js; plain "settings" is their menu.
	"settings:general", "settings:appearance", "settings:menubar", "settings:layout", "settings:alerts", "settings:shortcuts",
}

// Message is any JS→Go or shell→Go message; the comments on the fields say which types carry them.
type Message struct {
	Type string `json:"type"`
	// Name and Mode belong to "tab"; Name is one of the Tab constants or a plain tab name.
	// Name also carries the "hotkey" pressed, the app of an "app_info", a "proc_detail" or a
	// "browser_tabs" and, once parsed, the tab a "url" asks for.
	Name string `json:"name"`
	Mode string `json:"mode"`
	// App, PIDs and Force belong to "quit_app"; so does Signal, a key of Signals that is sent
	// to the one pid in place of a quit.
	App    string  `json:"app"`
	PIDs   []int32 `json:"pids"`
	Force  bool    `json:"force"`
	Signal string  `json:"signal"`
	// Key and Value belong to "set"; the settings store validates them.
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
	// Metric and Range belong to "history"; the history store validates them.
	Metric string `json:"metric"`
	Range  string `json:"range"`
	// Enabled belongs to "login_item" and "pin".
	Enabled bool `json:"enabled"`
	// Message belongs to "log".
	Message string `json:"message"`
	// Popover, Window and Pinned belong to "visibility".
	Popover bool `json:"popover"`
	Window  bool `json:"window"`
	Pinned  bool `json:"pinned"`
	// Mount belongs to "eject" and "reveal"; PID to "reveal", to "app_info", where 0 asks about
	// the app itself, and to "proc_detail". Path belongs to "storage_open" and "reveal": a
	// node of the scanned tree, relative to its root. A "reveal" carries exactly one of the three.
	Mount string `json:"mount"`
	PID   int32  `json:"pid"`
	Path  string `json:"path"`
	// Root belongs to "storage_scan": "home", or "choose" for a folder the user picks in a dialog.
	Root string `json:"root"`
	// Category belongs to "cleanup_list" and "cleanup": an id of storage.Categories. The rest
	// belongs to "cleanup": Review is the id of the review it confirms, which is only ever
	// compared with the one Go gave out, Permanent deletes for good instead of moving to the
	// Trash, Items are the entries of the review the user left checked, by name, and Rest is
	// its last row, every entry the review did not list by name.
	Category  string   `json:"category"`
	Review    string   `json:"review"`
	Permanent bool     `json:"permanent"`
	Items     []string `json:"items"`
	Rest      bool     `json:"rest"`
	// Args belongs to "proc_detail": read the command line too.
	Args bool `json:"args"`
	// Target belongs to "open": a key of OpenURLs.
	Target string `json:"target"`
	// URL belongs to "url": the mac-pulse:// link the bundle was opened with.
	URL string `json:"url"`
	// ID belongs to "alert_detail": the id of an alert of the state.
	ID string `json:"id"`
}

// ParseMessage is the trust boundary for the web view: whatever page content runs
// there can post anything, so a message with an unknown type or a field outside its
// domain is rejected whole.
func ParseMessage(raw []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrBadMessage, err)
	}
	switch msg.Type {
	case "tab":
		if !slices.Contains(tabs, msg.Name) || (msg.Mode != "popover" && msg.Mode != "window") {
			return Message{}, fmt.Errorf("%w: tab %q in mode %q", ErrBadMessage, msg.Name, msg.Mode)
		}
	case "set":
		if msg.Key == "" || len(msg.Value) == 0 {
			return Message{}, fmt.Errorf("%w: set without key or value", ErrBadMessage)
		}
	case "log":
		// A cut through the middle of a rune would put invalid UTF-8 in the log.
		msg.Message = strings.ToValidUTF8(msg.Message[:min(len(msg.Message), maxLogBytes)], "")
	default:
		if err := checkAction(&msg); err != nil {
			return Message{}, err
		}
	}
	return msg, nil
}

// checkAction covers the messages that act on one target or carry no fields at all. Only
// the shape is checked here; the session then takes nothing but a volume or a process of
// its last sample.
func checkAction(msg *Message) error {
	switch msg.Type {
	case "eject", "reveal":
		return checkTarget(msg)
	case "quit_app", "app_info", "proc_detail", "browser_tabs":
		return checkProcess(msg)
	case "storage_scan", "storage_open", "cleanup_list", "cleanup":
		return checkStorage(msg)
	case "open":
		if _, ok := OpenURLs[msg.Target]; !ok {
			return fmt.Errorf("%w: open of %.50q", ErrBadMessage, msg.Target)
		}
	case "url":
		if msg.Name = linkTab(msg.URL); msg.Name == "" {
			return fmt.Errorf("%w: url %.200q", ErrBadMessage, msg.URL)
		}
	// The id only picks a record, and one that picks none is answered, not dropped: the page
	// waits for that answer and finds it by the id, so only the length is bounded here.
	case "alert_detail":
		msg.ID = strings.ToValidUTF8(msg.ID[:min(len(msg.ID), maxAlertID)], "")
	case "hotkey":
		if msg.Name != "quit" {
			return fmt.Errorf("%w: hotkey %q", ErrBadMessage, msg.Name)
		}
	// public_ip, ping, export, copy, export_csv, speedtest, update_check and the three below
	// them carry no fields: the address, the paths and the text are Go's choice.
	case "history", "login_item", "open_window", "open_panel", "quit", "visibility", "public_ip", "ping", "export", "copy",
		"export_csv", "speedtest", "pin", "update_check", "storage_cancel", "storage_clear", "cleanup_scan":
	default:
		return fmt.Errorf("%w: unknown type %q", ErrBadMessage, msg.Type)
	}
	return nil
}

// checkTarget is checkAction for the messages that name a volume or something to show in Finder, split for gocyclo.
func checkTarget(msg *Message) error {
	switch msg.Type {
	case "eject":
		if !strings.HasPrefix(msg.Mount, "/Volumes/") || !cleanMount(msg.Mount) {
			return fmt.Errorf("%w: eject of %q", ErrBadMessage, msg.Mount)
		}
	case "reveal":
		targets := 0
		for _, set := range []bool{msg.Mount != "", msg.PID != 0, msg.Path != ""} {
			if set {
				targets++
			}
		}
		if targets != 1 || msg.PID < 0 || (msg.Mount != "" && !cleanMount(msg.Mount)) || !cleanRel(msg.Path) {
			return fmt.Errorf("%w: reveal of mount %q, pid %d and path %.300q", ErrBadMessage, msg.Mount, msg.PID, msg.Path)
		}
	}
	return nil
}

// checkProcess is checkAction for the messages that name an app or its processes.
func checkProcess(msg *Message) error {
	if msg.Type == "quit_app" {
		if msg.App == "" || len(msg.PIDs) == 0 || len(msg.PIDs) > maxQuitPIDs {
			return fmt.Errorf("%w: quit_app for %q with %d pids", ErrBadMessage, msg.App, len(msg.PIDs))
		}
		if _, known := Signals[msg.Signal]; msg.Signal != "" && (!known || len(msg.PIDs) != 1 || msg.Force) {
			return fmt.Errorf("%w: quit_app with signal %.20q for %d pids", ErrBadMessage, msg.Signal, len(msg.PIDs))
		}
		return nil
	}
	// Only a proc_detail must name a process; a browser_tabs names the app alone.
	if msg.Name == "" || len(msg.Name) > maxNameBytes || msg.PID < 0 || (msg.Type == "proc_detail" && msg.PID == 0) {
		return fmt.Errorf("%w: %s of %.300q pid %d", ErrBadMessage, msg.Type, msg.Name, msg.PID)
	}
	return nil
}

// checkStorage is checkAction for the Storage tab's messages. A path only ever picks a node
// of the tree Go scanned, and a category one of Go's own list.
func checkStorage(msg *Message) error {
	switch msg.Type {
	case "storage_scan":
		if msg.Root != "home" && msg.Root != "choose" {
			return fmt.Errorf("%w: storage_scan of %.50q", ErrBadMessage, msg.Root)
		}
	case "storage_open":
		if !cleanRel(msg.Path) {
			return fmt.Errorf("%w: storage_open of %.300q", ErrBadMessage, msg.Path)
		}
	case "cleanup_list", "cleanup":
		categories := storage.Categories("")
		i := slices.IndexFunc(categories, func(c storage.Category) bool { return c.ID == msg.Category })
		// What cannot go to the Trash is removed only by a message that says "for good".
		if i < 0 || (msg.Type == "cleanup" && categories[i].Permanent && !msg.Permanent) {
			return fmt.Errorf("%w: %s of %.50q, permanent %t", ErrBadMessage, msg.Type, msg.Category, msg.Permanent)
		}
		if msg.Type == "cleanup" && !cleanItems(msg.Items, msg.Rest) {
			return fmt.Errorf("%w: cleanup of %d items, rest %t", ErrBadMessage, len(msg.Items), msg.Rest)
		}
	}
	return nil
}

// cleanItems is true for a selection a review can produce: something is selected, no more
// names than a review lists, and every one the bare name of an entry, never a path.
func cleanItems(items []string, rest bool) bool {
	if (len(items) == 0 && !rest) || len(items) > maxReviewItems {
		return false
	}
	return !slices.ContainsFunc(items, func(name string) bool {
		return name == "" || name == "." || name == ".." || len(name) > maxMountBytes || strings.ContainsAny(name, "/\x00")
	})
}

// linkTab is the tab a mac-pulse://open?tab=<tab> link names, "" for any other link: it
// comes from whatever app or web page opened it.
func linkTab(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "mac-pulse" || u.Host != "open" || u.Path != "" || !slices.Contains(tabs, u.Query().Get("tab")) {
		return ""
	}
	return u.Query().Get("tab")
}

// cleanRel is true for "" and for a relative path that stays below its root, in the one spelling Go builds.
func cleanRel(rel string) bool {
	return rel == "" || (len(rel) <= maxMountBytes && filepath.IsLocal(rel) && filepath.Clean(rel) == rel)
}

// cleanMount is true for an absolute path with no "..", "." or doubled slash to climb out with.
func cleanMount(mount string) bool {
	return len(mount) <= maxMountBytes && filepath.IsAbs(mount) && filepath.Clean(mount) == mount
}

// QuitTargets picks the pids of a quit_app message that may be signalled. apps must be
// a fresh scan: the pids were shown seconds ago, and one may have exited or been reused
// since. A pid that is gone is dropped silently; every other refusal comes back as a
// sentence for the user.
func QuitTargets(apps []collector.App, msg Message, self Self) (targets []int32, refused []Text) {
	owners := map[int32]string{}
	procs := map[int32]collector.Process{}
	for _, app := range apps {
		for _, p := range app.Processes {
			owners[p.PID], procs[p.PID] = app.Name, p
		}
	}
	seen := map[int32]bool{}
	for _, pid := range msg.PIDs {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		owner, alive := owners[pid]
		switch {
		// kill(2) takes 0 and negative pids as "a whole process group" and -1 as "everyone".
		case pid <= 1:
			refused = append(refused, Text{Key: "quit.system_pid", Params: map[string]any{"pid": pid}})
		case pid == self.PID:
			refused = append(refused, Text{Key: "quit.self"})
		case !alive:
		// The frontend sends the name it was shown, which has the bidi controls removed.
		case displayText(owner) != msg.App:
			refused = append(refused, Text{Key: "quit.moved", Params: map[string]any{"pid": pid, "owner": displayText(owner), "app": msg.App}})
		case procs[pid].UID != self.UID:
			refused = append(refused, Text{Key: "quit.not_owned", Params: map[string]any{"pid": pid, "name": displayText(procs[pid].Name)}})
		case !killable(procs[pid], self):
			refused = append(refused, Text{Key: "quit.system", Params: map[string]any{"pid": pid, "name": displayText(procs[pid].Name)}})
		default:
			targets = append(targets, pid)
		}
	}
	return targets, refused
}

// RevealPath is what Finder shows for a reveal message's pid: the bundle of its app, or the
// executable of a process outside any bundle. apps is the last sample; "" refuses a pid that
// is not in it, belongs to another user or has no absolute path.
func RevealPath(apps []collector.App, pid int32, self Self) string {
	for _, app := range apps {
		for _, p := range app.Processes {
			if p.PID != pid {
				continue
			}
			path := cmp.Or(app.BundlePath, p.Exe)
			if p.UID != self.UID || !filepath.IsAbs(path) {
				return ""
			}
			return path
		}
	}
	return ""
}

// InfoTarget is what an app_info message asks about, out of the last sample: pid 0 is the app
// itself, any other pid one of its processes. The message only picks; path, the one to read,
// is Go's own, and info.Path is that path made safe to show. ok is false once the app or the
// process is gone.
func InfoTarget(apps []collector.App, msg Message, self Self) (info AppInfo, target collector.Process, path string, ok bool) {
	// The frontend sends the name it was shown, which has the bidi controls removed.
	i := slices.IndexFunc(apps, func(a collector.App) bool { return displayText(a.Name) == msg.Name })
	if i < 0 || len(apps[i].Processes) == 0 {
		return AppInfo{}, collector.Process{}, "", false
	}
	app := apps[i]
	// The app itself is the process its other processes descend from.
	target = app.Processes[0]
	for _, p := range app.Processes {
		if p.PID == msg.PID || msg.PID == 0 && !slices.Contains(app.PIDs, p.PPID) {
			target = p
			break
		}
	}
	info, path = AppInfo{Name: msg.Name, PID: msg.PID, Title: msg.Name}, cmp.Or(app.BundlePath, target.Exe)
	info.Killable = slices.ContainsFunc(app.Processes, func(p collector.Process) bool { return killable(p, self) })
	if msg.PID != 0 {
		if target.PID != msg.PID {
			return AppInfo{}, collector.Process{}, "", false
		}
		// A helper is a bundle of its own inside the app's.
		info.Title, path, info.Killable = displayText(target.Name), target.Exe, killable(target, self)
		if end := strings.LastIndex(target.Exe, ".app/"); end >= 0 {
			path = target.Exe[:end+len(".app")]
		}
	}
	info.Path, info.DescKey = displayText(path), processKey(info.Title)
	// Nothing on disk to read a signature from, and nothing started the kernel.
	if target.PID == 0 {
		info.Signing = collector.SignApple
		return info, target, path, true
	}
	info.Parent = startedBy(apps, target.PPID)
	return info, target, path, true
}

// DetailTarget is the process a proc_detail message asks about, out of the last sample. Only
// a process of this user's own is one: the message must not make Go read anybody else's.
func DetailTarget(apps []collector.App, msg Message, self Self) (target collector.Process, ok bool) {
	_, target, _, ok = InfoTarget(apps, msg, self)
	return target, ok && target.UID == self.UID
}

// BrowserRunning is true while the last sample has the app a browser_tabs message names
// with a process. It only spares a script that would find nothing: the script itself asks
// the browser by its bundle id and only while that one runs, so a look-alike of the same
// name gets no browser launched.
func BrowserRunning(apps []collector.App, name string) bool {
	return slices.ContainsFunc(apps, func(a collector.App) bool { return a.Name == name && len(a.Processes) > 0 })
}

// startedBy names the process pid, with its app when that goes by another name: "zsh (iTerm)".
func startedBy(apps []collector.App, pid int32) string {
	for _, a := range apps {
		for _, p := range a.Processes {
			if p.PID != pid {
				continue
			}
			if name, owner := displayText(p.Name), displayText(a.Name); name != owner {
				return name + " (" + owner + ")"
			}
			return displayText(p.Name)
		}
	}
	return ""
}

// EjectVolume is the volume an eject message's mount picks, the zero Volume when the last
// sample holds no ejectable one mounted there. The page sends the mount it was shown, which
// has the bidi controls removed; the volume carries the mount as the system spells it.
func EjectVolume(volumes []collector.Volume, mount string) collector.Volume {
	for _, v := range volumes {
		if v.Ejectable && displayText(v.Mount) == mount {
			return v
		}
	}
	return collector.Volume{}
}

// BuildProcDetail is the answer to a proc_detail: what was read of target, and the names of
// its parents out of the sample apps, from the nearest one up to launchd.
func BuildProcDetail(apps []collector.App, target collector.Process, d collector.ProcDetail) *ProcDetail {
	parents := map[int32]collector.Process{}
	for _, a := range apps {
		for _, p := range a.Processes {
			parents[p.PID] = p
		}
	}
	out := &ProcDetail{Threads: d.Threads, Chain: []string{}, Files: d.Files, Sockets: d.Sockets, More: d.More, Args: d.Args}
	// The cap ends a parent cycle, which a pid reused between two reads can make.
	for pid := target.PPID; pid > 0 && len(out.Chain) < maxChain; {
		p, ok := parents[pid]
		if !ok {
			break
		}
		out.Chain = append(out.Chain, displayText(p.Name))
		pid = p.PPID
	}
	return out
}
