package netinspect

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/kl09/mac-pulse/internal/collector"
)

const execTimeout = 3 * time.Second

// Inspector owns the previous poll's counters; only its Run goroutine touches them.
type Inspector struct {
	interval time.Duration
	onReport func(*Report)
	resolver *resolver
	// resolve is set from the message goroutine and read by Run's.
	resolve   atomic.Bool
	prevAt    time.Time
	prevProcs map[int32]byteCount
	prevConns map[string]byteCount
	base      map[int32]byteCount
	apps      map[int32]appID
	groups    func(pid int32) (name, bundlePath string, ok bool)
}

type appID struct {
	name       string
	bundlePath string
}

// sample is one parsed poll with rates, totals baseline and names resolved;
// report aggregates it without any I/O.
type sample struct {
	procs     map[int32]byteCount
	base      map[int32]byteCount
	procRates map[int32]byteRate
	conns     []Conn
	connRates map[string]byteRate
	listening []Listener
	names     map[int32]string
	// bundles maps an app name to its .app directory.
	bundles map[string]string
	// dirs maps a listening pid to its working directory.
	dirs map[int32]string
}

func NewInspector(interval time.Duration, onReport func(*Report)) *Inspector {
	return &Inspector{
		interval: interval,
		onReport: onReport,
		resolver: newResolver(),
		base:     map[int32]byteCount{},
		apps:     map[int32]appID{},
	}
}

// SetResolve switches reverse DNS on or off from the next poll; off, hosts stay empty and
// no PTR query leaves the machine.
func (i *Inspector) SetResolve(on bool) {
	i.resolve.Store(on)
}

// SetGroups makes the inspector name a pid the way the Apps tab groups it: lookup answers
// from the sampler's last scan, on any goroutine, and a pid it does not know falls back to
// collector.AppOf. Call it before Run.
func (i *Inspector) SetGroups(lookup func(pid int32) (name, bundlePath string, ok bool)) {
	i.groups = lookup
}

func (i *Inspector) Run(ctx context.Context) {
	i.prevAt, i.prevProcs, i.prevConns = time.Time{}, nil, nil
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()
	for {
		rep, err := i.poll(ctx)
		// A result that arrives during shutdown is dropped: its consumer is already gone.
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Debug("net poll", "err", err)
		} else {
			i.onReport(rep)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Listening is one nettop pass for the listening sockets alone, working directories included.
func Listening(ctx context.Context) ([]Listener, error) {
	_, _, listening, err := readNettop(ctx)
	if err != nil {
		return nil, err
	}
	names := map[int32]string{}
	for _, l := range listening {
		if _, ok := names[l.PID]; ok {
			continue
		}
		if names[l.PID], _ = collector.AppOf(ctx, l.PID); names[l.PID] == "" {
			names[l.PID] = fmt.Sprintf("pid %d", l.PID)
		}
	}
	return report(sample{listening: listening, names: names, dirs: listenerDirs(ctx, listening)}).Listening, nil
}

func readNettop(ctx context.Context) (map[int32]byteCount, []Conn, []Listener, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nettop", "-n", "-L", "1", "-x", "-J", "bytes_in,bytes_out,state").Output()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("nettop: %w", err)
	}
	procs, conns, listening, err := parseNettop(bytes.NewReader(out))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse nettop: %w", err)
	}
	return procs, conns, listening, nil
}

func (i *Inspector) poll(ctx context.Context) (*Report, error) {
	procs, conns, listening, err := readNettop(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().Round(0)
	curConns := make(map[string]byteCount, len(conns))
	for _, c := range conns {
		curConns[c.key()] = byteCount{In: c.BytesIn, Out: c.BytesOut}
	}
	s := sample{
		procs:     procs,
		base:      i.base,
		procRates: rates(i.prevProcs, procs, now.Sub(i.prevAt)),
		conns:     conns,
		connRates: rates(i.prevConns, curConns, now.Sub(i.prevAt)),
		listening: listening,
		names:     make(map[int32]string, len(procs)),
		bundles:   map[string]string{},
	}
	hasRates := !i.prevAt.IsZero()
	i.prevAt, i.prevProcs, i.prevConns = now, procs, curConns

	// The pid→app cache is rebuilt per poll, so a pid that stays present keeps
	// its name for life; only a pid absent for one poll is re-resolved.
	apps := make(map[int32]appID, len(procs))
	for pid, b := range procs {
		if _, ok := i.base[pid]; !ok {
			i.base[pid] = b
		}
		app := appFor(ctx, pid, i.groups, i.apps)
		if app.name == "" {
			app.name = fmt.Sprintf("pid %d", pid)
		}
		apps[pid] = app
		s.names[pid] = app.name
		if app.bundlePath != "" {
			s.bundles[app.name] = app.bundlePath
		}
	}
	i.apps = apps
	// A reused pid must not inherit the dead process's baseline.
	for pid := range i.base {
		if _, ok := procs[pid]; !ok {
			delete(i.base, pid)
		}
	}

	s.dirs = listenerDirs(ctx, listening)

	if i.resolve.Load() {
		i.lookupHosts(ctx, conns)
	}

	rep := report(s)
	rep.HasRates = hasRates
	return rep, nil
}

// appFor names the app of a pid: the group the sampler's last scan put it in, so that one
// process has its CPU and its traffic under one name; for a pid that scan has not seen, the
// name it was given on an earlier poll, or its owner as read now.
func appFor(ctx context.Context, pid int32, groups func(int32) (string, string, bool), named map[int32]appID) appID {
	if groups != nil {
		if name, bundlePath, ok := groups(pid); ok {
			return appID{name: name, bundlePath: bundlePath}
		}
	}
	app, ok := named[pid]
	if !ok {
		app.name, app.bundlePath = collector.AppOf(ctx, pid)
	}
	return app
}

// lookupHosts fills Host with the reverse DNS name of every remote address.
func (i *Inspector) lookupHosts(ctx context.Context, conns []Conn) {
	ips := map[string]struct{}{}
	for _, c := range conns {
		ips[c.RemoteIP] = struct{}{}
	}
	// The first poll blocks up to ~1 s per 4 uncached public IPs (timeout ×
	// in-flight cap); drop the Wait and read cache-only if that ever hurts.
	var wg sync.WaitGroup
	for ip := range ips {
		wg.Go(func() { i.resolver.Lookup(ctx, ip) })
	}
	wg.Wait()
	for idx := range conns {
		conns[idx].Host = i.resolver.Lookup(ctx, conns[idx].RemoteIP)
	}
}

// listenerDirs reads the working directory of every listening pid, the home directory
// shortened to "~". Another user's pid answers EPERM; that row simply has no directory.
func listenerDirs(ctx context.Context, listening []Listener) map[int32]string {
	home, _ := os.UserHomeDir()
	dirs := make(map[int32]string, len(listening))
	for _, l := range listening {
		if _, ok := dirs[l.PID]; ok {
			continue
		}
		var dir string
		if p, err := process.NewProcessWithContext(ctx, l.PID); err == nil {
			dir, _ = p.CwdWithContext(ctx)
		}
		dirs[l.PID] = collector.TildePath(home, dir)
	}
	return dirs
}

func report(s sample) *Report {
	rep := &Report{Listening: s.listening}
	apps := map[string]*AppTraffic{}
	for pid, b := range s.procs {
		name := s.names[pid]
		a := apps[name]
		if a == nil {
			a = &AppTraffic{App: name, BundlePath: s.bundles[name]}
			apps[name] = a
		}
		a.PIDs = append(a.PIDs, pid)
		a.DownRate += s.procRates[pid].In
		a.UpRate += s.procRates[pid].Out
		a.DownTotal += b.In - min(b.In, s.base[pid].In)
		a.UpTotal += b.Out - min(b.Out, s.base[pid].Out)
	}
	talkers := map[string]*Talker{}
	for _, c := range s.conns {
		c.App = s.names[c.PID]
		c.DownRate, c.UpRate = s.connRates[c.key()].In, s.connRates[c.key()].Out
		rep.Conns = append(rep.Conns, c)
		if a := apps[c.App]; a != nil {
			a.Conns++
		}
		if c.RemoteIP == "*" {
			continue
		}
		t := talkers[c.RemoteIP]
		if t == nil {
			t = &Talker{IP: c.RemoteIP, Host: c.Host}
			talkers[c.RemoteIP] = t
		}
		if !slices.Contains(t.Apps, c.App) {
			t.Apps = append(t.Apps, c.App)
		}
		t.DownRate += c.DownRate
		t.UpRate += c.UpRate
		t.Conns++
	}
	for idx := range rep.Listening {
		l := &rep.Listening[idx]
		l.App = s.names[l.PID]
		if l.Dir = s.dirs[l.PID]; l.Dir == "/" {
			l.Dir = ""
		}
	}
	for _, a := range apps {
		slices.Sort(a.PIDs)
		rep.Apps = append(rep.Apps, *a)
	}
	for _, t := range talkers {
		slices.Sort(t.Apps)
		rep.Talkers = append(rep.Talkers, *t)
	}
	sort.Slice(rep.Apps, func(x, y int) bool {
		return byRate(rep.Apps[x].DownRate+rep.Apps[x].UpRate, rep.Apps[y].DownRate+rep.Apps[y].UpRate, rep.Apps[x].App, rep.Apps[y].App)
	})
	sort.Slice(rep.Talkers, func(x, y int) bool {
		return byRate(rep.Talkers[x].DownRate+rep.Talkers[x].UpRate, rep.Talkers[y].DownRate+rep.Talkers[y].UpRate,
			rep.Talkers[x].IP, rep.Talkers[y].IP)
	})
	sort.SliceStable(rep.Conns, func(x, y int) bool {
		return byRate(rep.Conns[x].DownRate+rep.Conns[x].UpRate, rep.Conns[y].DownRate+rep.Conns[y].UpRate, rep.Conns[x].App, rep.Conns[y].App)
	})
	sort.SliceStable(rep.Listening, func(x, y int) bool { return rep.Listening[x].App < rep.Listening[y].App })
	return rep
}

// Busiest first; the name tie-break keeps idle rows from jumping between ticks.
func byRate(rateX, rateY float64, nameX, nameY string) bool {
	if rateX != rateY {
		return rateX > rateY
	}
	return nameX < nameY
}
