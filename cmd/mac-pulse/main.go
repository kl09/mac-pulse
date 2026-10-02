package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/shell"
	"github.com/kl09/mac-pulse/internal/store"
	"github.com/kl09/mac-pulse/internal/ui"
	"github.com/kl09/mac-pulse/internal/ui/web"
)

const (
	visibleInterval = 2 * time.Second
	// With no view on screen only the menu bar, the history and the alerts consume samples.
	hiddenInterval = 5 * time.Second
	// nettop reports a socket's bytes only while it is open: what a socket moved after the
	// last poll is lost when it closes, so the poll is frequent (22 ms CPU each).
	usageInterval = 10 * time.Second
	// The Dev tab lists servers and containers, which come and go by the minute: its nettop
	// and docker polls (22 ms and 40 ms of CPU each) need not keep the pace of a rate chart.
	devInterval   = 10 * time.Second
	flushInterval = 5 * time.Minute
	// The first flush comes early: a crash in the first minutes must not cost the history
	// gathered since the start.
	firstFlush = time.Minute
	// MAC_PULSE_ACTION fires after the debug snapshot, so the PNG shows the screen before it;
	// the messages of a list follow actionStep apart, time enough for a measurement before a cleanup.
	actionDelay = 6 * time.Second
	actionStep  = 3 * time.Second
	// Two sampler ticks plus the page load, so the debug snapshot shows live numbers.
	snapshotDelay = 5 * time.Second
	// -json waits for the second process scan (the 4th tick) to have per-app rates; no need
	// to make that take 8 s.
	jsonInterval = 500 * time.Millisecond
	jsonTicks    = 4
)

// version is set by the Makefile through -ldflags "-X main.version=…".
var version = "dev"

func versionLine() string { return "mac-pulse " + version }

func main() {
	stateJSON := flag.Bool("json", false, "print one state as JSON and exit")
	storageDir := flag.String("storage", "", "scan `folder` and print its top level as JSON, then exit")
	caches := flag.Bool("caches", false, "print the cleanup categories with their sizes as JSON and exit; removes nothing")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(versionLine())
		return
	}
	if os.Getenv("MAC_PULSE_DEBUG") != "" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	// Every collector shells out by bare name; a same-named file in ~/go/bin or Homebrew must not win.
	_ = os.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	var err error
	switch {
	case flag.Arg(0) == "mcp":
		err = runMCP(ctx)
	case *storageDir != "":
		err = printLevel(ctx, *storageDir)
	case *caches:
		err = printCaches(ctx)
	default:
		err = run(ctx, cancel, *stateJSON)
	}
	cancel()
	if err != nil {
		slog.Error("mac-pulse", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, quit func(), stateJSON bool) error {
	base, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate Application Support: %w", err)
	}
	dir := filepath.Join(base, "mac-pulse")
	self := ui.Self{UID: uint32(os.Getuid()), PID: int32(os.Getpid())}
	if stateJSON {
		return printState(ctx, dir, self)
	}
	// A first launch adds nothing but a number to the menu bar: show the panel once. The
	// lock file outlives every run, while settings.json appears only after a Set.
	_, statErr := os.Stat(filepath.Join(dir, "lock"))
	firstRun := errors.Is(statErr, fs.ErrNotExist)
	release, running, err := shell.LockInstance(dir)
	if err != nil {
		return fmt.Errorf("single-instance lock: %w", err)
	}
	if running {
		fmt.Fprintln(os.Stderr, "mac-pulse is already running; its panel has been opened")
		return nil
	}
	defer release()
	cfg, err := settings.Open(dir)
	if err != nil {
		return fmt.Errorf("open settings: %w", err)
	}
	history, err := store.Open(dir)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}

	sess := &session{
		quit: quit,
		self: self,
		// Like MAC_PULSE_ACTION, a debug hook no plain build obeys.
		statusFixed: os.Getenv("MAC_PULSE_DEBUG") != "" && os.Getenv("MAC_PULSE_STATUS_FIXED") != "",
		systemLang:  settings.MatchLanguage(shell.PreferredLanguages()),
		settings:    cfg,
		history:     history,
		latest:      &latest{},
		home:        homeDir(),
		tabs:        map[string]string{},
		visible:     map[string]bool{},
		noTrash:     map[string]bool{},
	}
	sess.alerts = alerts.New(sess.notifyAlert, history.Alerts())
	sess.applySettings()
	sess.sampler = collector.NewSampler(hiddenInterval, sess.onSample)
	sess.inspector = netinspect.NewInspector(visibleInterval, sess.onNet)
	sess.devInspector = netinspect.NewInspector(devInterval, sess.onNet)
	sess.docker = collector.NewDocker(devInterval, sess.onDocker)
	usage := netinspect.NewUsage(usageInterval, history.AddUsage)
	sess.inspector.SetGroups(sess.group)
	sess.devInspector.SetGroups(sess.group)
	usage.SetGroups(sess.group)

	var wg sync.WaitGroup
	wg.Go(func() { sess.sampler.Run(ctx) })
	wg.Go(func() { usage.Run(ctx) })
	wg.Go(func() { flushEvery(ctx, sess.flush) })
	wg.Go(func() {
		<-ctx.Done()
		shell.Quit()
	})
	// Like the other hooks, off without MAC_PULSE_DEBUG: no plain build writes a file because of its environment.
	if path := os.Getenv("MAC_PULSE_SNAPSHOT"); path != "" && os.Getenv("MAC_PULSE_DEBUG") != "" {
		wg.Go(func() { snapshotTo(ctx, path) })
	}
	wg.Go(func() { debugAction(ctx) })

	shell.Run(shell.Options{
		Assets:    web.FS,
		OnMessage: func(raw []byte) { sess.onMessage(ctx, raw) },
		Tab:       os.Getenv("MAC_PULSE_TAB"),
		Version:   version,
		Open:      firstRun || os.Getenv("MAC_PULSE_OPEN") != "",
		Window:    os.Getenv("MAC_PULSE_WINDOW") != "",
	})
	quit()
	sess.stopInspector()
	sess.stopDocker()
	sess.lookups.Wait()
	wg.Wait()
	if err := sess.flush(); err != nil {
		return fmt.Errorf("flush history: %w", err)
	}
	return nil
}

func flushEvery(ctx context.Context, flush func() error) {
	timer := time.NewTimer(firstFlush)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := flush(); err != nil {
				slog.Error("flush history", "err", err)
			}
			timer.Reset(flushInterval)
		}
	}
}

// debugAction is the MAC_PULSE_ACTION debug hook, off without MAC_PULSE_DEBUG so that no
// build makes a network request nobody clicked for. The visible view posts the messages
// itself, so they take the frontend's path.
func debugAction(ctx context.Context) {
	if os.Getenv("MAC_PULSE_DEBUG") == "" {
		return
	}
	wait := actionDelay
	for _, msg := range debugMessages(os.Getenv("MAC_PULSE_ACTION")) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			shell.Push("send", msg)
		}
		wait = actionStep
	}
}

// debugMessages reads the value of MAC_PULSE_ACTION: the name of a message without fields,
// a whole message as JSON, or a JSON array of messages. Push evaluates its payload as
// JavaScript, so nothing but JSON comes back.
func debugMessages(action string) []json.RawMessage {
	if slices.Contains([]string{"public_ip", "ping", "export", "copy", "export_csv", "speedtest"}, action) {
		return []json.RawMessage{json.RawMessage(`{"type":"` + action + `"}`)}
	}
	var list []json.RawMessage
	if json.Unmarshal([]byte(action), &list) == nil {
		return list
	}
	if json.Valid([]byte(action)) {
		return []json.RawMessage{json.RawMessage(action)}
	}
	return nil
}

// snapshotTo is the MAC_PULSE_SNAPSHOT debug hook: one PNG shortly after start and one per SIGUSR1.
func snapshotTo(ctx context.Context, path string) {
	usr1 := make(chan os.Signal, 1)
	signal.Notify(usr1, syscall.SIGUSR1)
	defer signal.Stop(usr1)
	first := time.After(snapshotDelay)
	for {
		select {
		case <-ctx.Done():
			return
		case <-first:
		case <-usr1:
		}
		shell.Snapshot(path)
	}
}

func printState(ctx context.Context, dir string, self ui.Self) error {
	// Every detail collector runs, as if all the detail screens were open at once.
	state, err := sampleState(ctx, dir, self, collector.Detail{Temps: true, Bluetooth: true, NetInfo: true, SMART: true, Dev: true})
	if err != nil {
		return err
	}
	return printJSON(state)
}

// homeDir is the folder the storage code scans and cleans, "" when it is not to be trusted;
// paths then stay unshortened and the Storage tab refuses its messages.
func homeDir() string {
	account, err := user.Current()
	if err != nil {
		slog.Warn("locate home directory", "err", err)
		return ""
	}
	home := storageHome(account.HomeDir, os.Getenv("HOME"), os.Getenv("MAC_PULSE_DEBUG") != "")
	if home == "" {
		slog.Warn("no usable home directory: storage is off", "account", account.HomeDir, "env", os.Getenv("HOME"))
	}
	return home
}

// storageHome picks the home folder for the code that deletes: the one of the user's
// account, never what the environment claims, since HOME=/ would make /Library/Caches a
// cleanup category. A HOME that differs switches storage off; only a debug run takes it, so
// that a scratch home can be cleaned in a test without touching the real one. "/" and a
// relative path are no home.
func storageHome(account, env string, debug bool) string {
	home := account
	switch {
	case env == account:
	case debug:
		home = env
	default:
		return ""
	}
	if !filepath.IsAbs(home) || filepath.Clean(home) == "/" {
		return ""
	}
	return filepath.Clean(home)
}

// sampleState builds one state without taking the instance lock: it only reads the
// settings and the history, and what it adds to the history is never flushed. detail.Dev
// adds what the Dev tab shows, at the price of `docker stats`.
func sampleState(ctx context.Context, dir string, self ui.Self, detail collector.Detail) (ui.State, error) {
	cfg, err := settings.Open(dir)
	if err != nil {
		return ui.State{}, fmt.Errorf("open settings: %w", err)
	}
	history, err := store.OpenReadOnly(dir)
	switch {
	case errors.Is(err, store.ErrUndecodable):
		slog.Warn("history left out of the state", "err", err)
	case err != nil:
		return ui.State{}, fmt.Errorf("open history: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	in := ui.Input{
		Settings: cfg.Get(), SystemLang: settings.MatchLanguage(shell.PreferredLanguages()),
		LoginItem: shell.LoginItem(), Self: self, AllApps: true, Home: homeDir(),
		// Nothing is scanned or measured for a state: the storage part comes with its idle values.
		Storage: &ui.StorageInput{},
	}
	// The docker CLI and nettop answer while the sampler ticks.
	var slow sync.WaitGroup
	if detail.Dev {
		slow.Go(func() { in.Docker = collector.ReadDocker(ctx) })
		slow.Go(func() {
			listening, err := netinspect.Listening(ctx)
			if err != nil {
				slog.Warn("read listening sockets", "err", err)
			}
			in.Listening = listening
		})
	}
	samples := make(chan *collector.Snapshot)
	sampler := collector.NewSampler(jsonInterval, func(s *collector.Snapshot) {
		select {
		case samples <- s:
		case <-ctx.Done():
		}
	})
	sampler.SetDetail(detail)
	go sampler.Run(ctx)
	for range jsonTicks {
		select {
		case in.Snap = <-samples:
			history.Add(in.Snap)
		case <-ctx.Done():
			return ui.State{}, fmt.Errorf("wait for sample: %w", ctx.Err())
		}
	}
	slow.Wait()
	in.Spark, in.Today, in.NetTotals = history.Spark(), history.Today(), history.NetTotals()
	return ui.Build(in), nil
}
