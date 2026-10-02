package collector

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/kl09/mac-pulse/internal/native"
)

// 64 hops caps a ppid cycle; the real chain is < 10.
const maxParentHops = 64

// AppOf names the app a pid belongs to and its .app directory; bundlePath is ""
// for a process outside any bundle, and both are "" once the process is gone.
func AppOf(ctx context.Context, pid int32) (name, bundlePath string) {
	owner, bundlePath, _, ok := ownerOf(pid, func(cur int32) (Process, bool) {
		p, err := process.NewProcessWithContext(ctx, cur)
		if err != nil {
			return Process{}, false
		}
		pr := Process{PID: cur}
		pr.Name, _ = p.NameWithContext(ctx)
		pr.Exe, _ = p.ExeWithContext(ctx)
		pr.PPID, _ = p.PpidWithContext(ctx)
		return pr, true
	})
	if !ok {
		return "", ""
	}
	if bundlePath != "" {
		return appName(bundlePath), bundlePath
	}
	// The name the process list shows: ps prints the exec path as invoked ("codex"),
	// while the kernel comm follows the symlink ("2.1.285"). 2 ms, once per new pid.
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "pid=,ppid=,uid=,tty=,time=,rss=,comm=", "-p", strconv.Itoa(int(owner.PID))).Output()
	if procs := parsePS(bytes.NewReader(out)); err == nil && len(procs) == 1 {
		return procs[0].Name, ""
	}
	return owner.Name, ""
}

// Apps scans the processes once and groups them. There is no earlier scan to diff
// against, so every CPU is 0.
func Apps(ctx context.Context) ([]App, error) {
	procs, err := readProcesses(ctx, nil, 0)
	if err != nil {
		return nil, err
	}
	return groupByApp(procs), nil
}

// readProcesses goes through ps (setuid root): proc_pidinfo answers EPERM for another
// user's pid without root, and gopsutil then reports zero CPU and memory with no error.
func readProcesses(ctx context.Context, prevTimes map[int32]float64, dt float64) ([]Process, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,uid=,tty=,time=,rss=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	procs := parsePS(bytes.NewReader(out))
	for i, p := range procs {
		// ps prints the path as invoked: "pmset", "head" and "cloudphotod" carry no directory,
		// and the system flag is decided by it.
		if !filepath.IsAbs(p.Exe) {
			if exe, err := (&process.Process{Pid: p.PID}).ExeWithContext(ctx); err == nil && exe != "" {
				procs[i].Exe = exe
			}
		}
		if prev, ok := prevTimes[p.PID]; ok && dt > 0 && p.CPUTime >= prev {
			procs[i].CPU = (p.CPUTime - prev) / dt * 100
		}
	}
	return procs, nil
}

// parsePS reads `ps -axo pid=,ppid=,uid=,tty=,time=,rss=,comm=`; comm is last because it may hold spaces.
func parsePS(r io.Reader) []Process {
	var procs []Process
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		rest := sc.Text()
		var cols [6]string
		for i := range cols {
			rest = strings.TrimLeft(rest, " ")
			j := strings.IndexByte(rest, ' ')
			if j < 0 {
				break
			}
			cols[i], rest = rest[:j], rest[j+1:]
		}
		pid, err := strconv.ParseInt(cols[0], 10, 32)
		if err != nil || cols[5] == "" {
			continue
		}
		ppid, _ := strconv.ParseInt(cols[1], 10, 32)
		uid, _ := strconv.ParseUint(cols[2], 10, 32)
		rss, _ := strconv.ParseUint(cols[5], 10, 64)
		exe := strings.TrimLeft(rest, " ")
		procs = append(procs, Process{
			PID: int32(pid), PPID: int32(ppid), UID: uint32(uid), TTY: strings.Trim(cols[3], "?"), Name: filepath.Base(exe), Exe: exe,
			CPUTime: parseCPUTime(cols[4]), RSS: rss * 1024,
		})
	}
	return procs
}

// parseCPUTime reads the TIME column of ps and top: [d-][[hh:]mm:]ss[.hh]; unreadable is 0.
func parseCPUTime(s string) float64 {
	var days float64
	if d, rest, ok := strings.Cut(s, "-"); ok {
		days, _ = strconv.ParseFloat(d, 64)
		s = rest
	}
	var seconds, unit float64 = 0, 1
	for _, part := range slices.Backward(strings.Split(s, ":")) {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0
		}
		seconds += v * unit
		unit *= 60
	}
	return days*86400 + seconds
}

// readKernelTask reads pid 0, which ps never prints, through top. It costs ~0.13 s CPU,
// so the sampler calls it only every kernelEvery ticks and keeps the last reading.
func readKernelTask(ctx context.Context) (Process, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "top", "-l", "1", "-pid", "0", "-stats", "pid,time,mem").Output()
	if err != nil {
		return Process{}, fmt.Errorf("top kernel_task: %w", err)
	}
	return parseTop(bytes.NewReader(out))
}

// parseTop finds the "0  <time>  <mem>" row of `top -l 1 -pid 0 -stats pid,time,mem`.
func parseTop(r io.Reader) (Process, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 || f[0] != "0" {
			continue
		}
		// top prints memory as 39M, with a trailing + or - for a change since the last sample.
		units := map[string]uint64{"K": 1 << 10, "M": 1 << 20, "G": 1 << 30}
		size := strings.TrimRight(f[2], "+-")
		unit := units[strings.TrimLeft(size, "0123456789")]
		mem, err := strconv.ParseUint(strings.TrimRight(size, "KMG"), 10, 64)
		if err != nil {
			return Process{}, fmt.Errorf("top kernel_task mem %q: %w", f[2], err)
		}
		return Process{Name: "kernel_task", CPUTime: parseCPUTime(f[1]), RSS: mem * max(unit, 1)}, nil
	}
	return Process{}, errors.New("top: no row for pid 0")
}

// usageRates turns two cumulative readings into bytes per second and average milliwatts.
// A counter that went down is a reused pid: the readings belong to two processes.
//
// A reused pid whose new process is already past the old counters shows one
// wrong scan; compare start times if that spike is ever seen.
func usageRates(prev, cur native.Usage, dt float64) (readRate, writeRate, energyMW float64) {
	if dt <= 0 || cur.DiskRead < prev.DiskRead || cur.DiskWrite < prev.DiskWrite || cur.EnergyNJ < prev.EnergyNJ {
		return 0, 0, 0
	}
	return float64(cur.DiskRead-prev.DiskRead) / dt, float64(cur.DiskWrite-prev.DiskWrite) / dt,
		float64(cur.EnergyNJ-prev.EnergyNJ) / dt / 1e6
}

func groupByApp(procs []Process) []App {
	names, bundles := groupOwners(procs)
	groups := map[string]*App{}
	for i, p := range procs {
		name, bundlePath := names[i], bundles[i]
		g := groups[name]
		if g == nil {
			g = &App{Name: name}
			groups[name] = g
		}
		// A bare binary that shares the bundle's name may have opened the group.
		if g.BundlePath == "" {
			g.BundlePath = bundlePath
		}
		g.PIDs = append(g.PIDs, p.PID)
		g.CPU += p.CPU
		g.RSS += p.RSS
		g.HasUsage = g.HasUsage || p.HasUsage
		g.DiskReadRate += p.DiskReadRate
		g.DiskWriteRate += p.DiskWriteRate
		g.EnergyMW += p.EnergyMW
		g.Processes = append(g.Processes, p)
	}
	apps := make([]App, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.Processes, func(i, j int) bool { return g.Processes[i].CPU > g.Processes[j].CPU })
		apps = append(apps, *g)
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].CPU != apps[j].CPU {
			return apps[i].CPU > apps[j].CPU
		}
		if apps[i].RSS != apps[j].RSS {
			return apps[i].RSS > apps[j].RSS
		}
		return apps[i].Name < apps[j].Name
	})
	return apps
}

// groupOwners names the group of every process and its .app directory, "" outside any bundle.
// Split from groupByApp for gocyclo.
func groupOwners(procs []Process) (names, bundles []string) {
	byPID := make(map[int32]Process, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	names, bundles = make([]string, len(procs)), make([]string, len(procs))
	ttyBundle, typed := map[string]string{}, map[string]bool{}
	for i, p := range procs {
		owner, bundlePath, cut, _ := ownerOf(p.PID, func(pid int32) (Process, bool) {
			parent, ok := byPID[pid]
			return parent, ok
		})
		names[i], bundles[i] = owner.Name, bundlePath
		switch {
		// A daemon outside any bundle is nobody's typed command: an app's child of the same
		// name stays with the app.
		case cut:
			typed[owner.Name] = true
		// The prompt's orphaned shell has no bundle and must not take the tty from its terminal.
		case p.TTY != "" && shell(p.Name):
			ttyBundle[p.TTY] = cmp.Or(bundlePath, ttyBundle[p.TTY])
		}
	}
	for i, p := range procs {
		switch {
		// A prompt's background shell is reparented to launchd; its terminal is the one
		// whose shell sits on the same tty.
		case bundles[i] == "" && shell(p.Name) && ttyBundle[p.TTY] != "":
			bundles[i] = ttyBundle[p.TTY]
		// One executable, one group: codex started by Chrome joins the codex typed in a shell.
		// Its children stay with the app; walk them too if such a helper ever has any.
		case bundles[i] != "" && typed[p.Name] && !shell(p.Name) && !strings.Contains(p.Exe, ".app/"):
			names[i], bundles[i] = p.Name, ""
		}
		if bundles[i] != "" {
			names[i] = appName(bundles[i])
		}
	}
	return names, bundles
}

// ownerOf returns the process that names pid's group and the group's .app directory, ""
// outside any bundle. A command typed in a shell (zsh → codex) owns itself and all it
// starts, and typed says the owner is such a command; everything else, shells included,
// belongs to the outermost bundle above it. The chain follows Responsible where a process
// has one.
//
// A shell an app runs a helper through (sh -c) counts as typed; require a tty
// on the shell if such helpers show up as rows.
func ownerOf(pid int32, lookup func(int32) (Process, bool)) (owner Process, bundlePath string, typed, ok bool) {
	chain := make([]Process, 0, 8)
	for range maxParentHops {
		p, found := lookup(pid)
		if !found {
			break
		}
		chain = append(chain, p)
		if pid = cmp.Or(p.Responsible, p.PPID); pid <= 1 {
			break
		}
	}
	if len(chain) == 0 {
		return Process{}, "", false, false
	}
	cut := false
	for i := len(chain) - 1; i > 0 && !cut; i-- {
		if shell(chain[i].Name) && !shell(chain[i-1].Name) {
			chain, cut = chain[:i], true
		}
	}
	for _, p := range slices.Backward(chain) {
		if i := strings.Index(p.Exe, ".app/"); i >= 0 {
			return p, p.Exe[:i+len(".app")], false, true
		}
		if cut {
			return p, "", true, true
		}
	}
	return chain[0], "", false, true
}

// shell reports a shell or its housekeeping: login shells are named "-zsh".
func shell(name string) bool {
	name = strings.TrimPrefix(name, "-")
	return strings.HasPrefix(name, "gitstatusd") ||
		slices.Contains([]string{"zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh", "csh", "login"}, name)
}

func appName(bundlePath string) string {
	return strings.TrimSuffix(filepath.Base(bundlePath), ".app")
}
