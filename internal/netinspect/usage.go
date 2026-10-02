package netinspect

import (
	"context"
	"log/slog"
	"maps"
	"net"
	"time"
)

// AppUsage is the traffic one app moved during one Usage interval.
type AppUsage struct {
	App string
	// BundlePath is the .app directory, "" for a process outside any bundle.
	BundlePath string
	Down       uint64
	Up         uint64
}

// Usage polls socket byte counters at a slow cadence, without rDNS, so per-app totals
// keep accumulating while the Inspector is stopped. Only its Run goroutine touches its state.
// A socket that opens and closes between two polls is never seen, and one that
// closes loses its last interval; a Network Extension is the only exact source.
type Usage struct {
	interval time.Duration
	onUsage  func([]AppUsage)
	// prev is nil until the first poll, which only sets the baseline.
	prev   map[flow]byteCount
	apps   map[int32]appID
	groups func(pid int32) (name, bundlePath string, ok bool)
}

type flow struct {
	pid int32
	key string
}

func NewUsage(interval time.Duration, onUsage func([]AppUsage)) *Usage {
	return &Usage{interval: interval, onUsage: onUsage, apps: map[int32]appID{}}
}

// SetGroups is Inspector.SetGroups for the traffic totals. Call it before Run.
func (u *Usage) SetGroups(lookup func(pid int32) (name, bundlePath string, ok bool)) {
	u.groups = lookup
}

func (u *Usage) Run(ctx context.Context) {
	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()
	for {
		usage, err := u.poll(ctx)
		// A result that arrives during shutdown is dropped: its consumer is already gone.
		if ctx.Err() != nil {
			return
		}
		switch {
		case err != nil:
			slog.Debug("net usage poll", "err", err)
		case len(usage) > 0:
			u.onUsage(usage)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (u *Usage) poll(ctx context.Context) ([]AppUsage, error) {
	procs, conns, _, err := readNettop(ctx)
	if err != nil {
		return nil, err
	}
	cur := flows(conns)
	deltas := flowDeltas(u.prev, cur)
	u.prev = cur

	// A reused pid must not inherit the dead process's name.
	maps.DeleteFunc(u.apps, func(pid int32, _ appID) bool {
		_, alive := procs[pid]
		return !alive
	})
	byApp := map[string]*AppUsage{}
	for pid, d := range deltas {
		app := appFor(ctx, pid, u.groups, u.apps)
		// Gone before it could be named: its bytes have no app to go to.
		if app.name == "" {
			continue
		}
		u.apps[pid] = app
		a := byApp[app.name]
		if a == nil {
			a = &AppUsage{App: app.name, BundlePath: app.bundlePath}
			byApp[app.name] = a
		}
		a.Down += d.In
		a.Up += d.Out
	}
	usage := make([]AppUsage, 0, len(byApp))
	for _, a := range byApp {
		usage = append(usage, *a)
	}
	return usage, nil
}

// flows indexes a poll's sockets by pid and endpoints. Loopback sockets are left out:
// traffic between two local processes is not network usage, and the machine-wide
// today counters skip lo0 as well. So is a socket without a pid, which nettop's broken
// frame leaves nobody's: pid 0 is the kernel in the process list, and would be named for it.
func flows(conns []Conn) map[flow]byteCount {
	cur := make(map[flow]byteCount, len(conns))
	for _, c := range conns {
		if c.PID <= 0 || net.ParseIP(c.RemoteIP).IsLoopback() {
			continue
		}
		f := flow{pid: c.PID, key: c.key()}
		cur[f] = byteCount{In: cur[f].In + c.BytesIn, Out: cur[f].Out + c.BytesOut}
	}
	return cur
}

// flowDeltas gives the bytes each pid moved since prev, leaving out idle pids. nettop's
// per-process counter is the sum over open sockets and drops when one closes, so the
// delta is taken per socket: one absent from prev, or below its previous reading (a new
// socket on the same 4-tuple), counts in full.
func flowDeltas(prev, cur map[flow]byteCount) map[int32]byteCount {
	deltas := map[int32]byteCount{}
	if prev == nil {
		return deltas
	}
	for f, c := range cur {
		if p := prev[f]; c.In >= p.In && c.Out >= p.Out {
			c = byteCount{In: c.In - p.In, Out: c.Out - p.Out}
		}
		if c == (byteCount{}) {
			continue
		}
		d := deltas[f.pid]
		deltas[f.pid] = byteCount{In: d.In + c.In, Out: d.Out + c.Out}
	}
	return deltas
}
