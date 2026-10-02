package collector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"golang.org/x/sys/unix"
)

// readTopology lists the efficiency cluster first: that is the order of the per-core
// array (cpu0–cpu3 are the E cores on an M4 Pro). An Intel Mac has no perflevels.
func readTopology() (model string, clusters []Cluster) {
	model, _ = unix.Sysctl("machdep.cpu.brand_string")
	levels, _ := unix.SysctlUint32("hw.nperflevels")
	for level := int(levels) - 1; level >= 0; level-- {
		name, err := unix.Sysctl(fmt.Sprintf("hw.perflevel%d.name", level))
		if err != nil {
			return model, nil
		}
		cores, err := unix.SysctlUint32(fmt.Sprintf("hw.perflevel%d.logicalcpu", level))
		if err != nil {
			return model, nil
		}
		clusters = append(clusters, Cluster{Name: name, Cores: int(cores)})
	}
	return model, clusters
}

// readCPU returns the load since prev together with the reading the next call diffs against.
func readCPU(ctx context.Context, prev cpu.TimesStat) (CPU, cpu.TimesStat, error) {
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return CPU{}, prev, fmt.Errorf("cpu times: %w", err)
	}
	if len(times) == 0 {
		return CPU{}, prev, errors.New("cpu times: empty")
	}
	perCore, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		return CPU{}, prev, fmt.Errorf("cpu percent per core: %w", err)
	}
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return CPU{}, prev, fmt.Errorf("load avg: %w", err)
	}
	uptime, err := host.UptimeWithContext(ctx)
	if err != nil {
		return CPU{}, prev, fmt.Errorf("uptime: %w", err)
	}
	user, system := cpuShares(prev, times[0])
	return CPU{
		Total:   user + system,
		User:    user,
		System:  system,
		PerCore: perCore,
		Load1:   avg.Load1,
		Load5:   avg.Load5,
		Load15:  avg.Load15,
		Uptime:  time.Duration(uptime) * time.Second,
	}, times[0], nil
}

// cpuShares gives user and system time between two readings as percent of all cores.
func cpuShares(prev, cur cpu.TimesStat) (user, system float64) {
	all := cur.User + cur.Nice + cur.System + cur.Idle - prev.User - prev.Nice - prev.System - prev.Idle
	if all <= 0 {
		return 0, 0
	}
	user = max(cur.User+cur.Nice-prev.User-prev.Nice, 0) / all * 100
	system = max(cur.System-prev.System, 0) / all * 100
	return user, system
}
