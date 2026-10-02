package collector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/shirou/gopsutil/v4/mem"
	"golang.org/x/sys/unix"
)

const (
	PressureNormal   = "normal"
	PressureWarning  = "warning"
	PressureCritical = "critical"
)

var (
	vmStatPageSize = regexp.MustCompile(`page size of (\d+) bytes`)
	vmStatLine     = regexp.MustCompile(`(?m)^"?([^":]+)"?:\s+(\d+)\.$`)
)

// readMemory keeps whatever it could read when a later source fails: the split
// comes from vm_stat, the rest from sysctl.
func readMemory(ctx context.Context) (Memory, error) {
	m := Memory{Pressure: PressureNormal}
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return m, fmt.Errorf("virtual memory: %w", err)
	}
	m.Total, m.Used, m.Wired, m.Free = vm.Total, vm.Used, vm.Wired, vm.Free
	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return m, fmt.Errorf("swap memory: %w", err)
	}
	m.SwapUsed, m.SwapTotal = swap.Used, swap.Total
	level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if err != nil {
		return m, fmt.Errorf("sysctl memory pressure: %w", err)
	}
	switch level {
	case 2:
		m.Pressure = PressureWarning
	case 4:
		m.Pressure = PressureCritical
	}
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vm_stat").Output()
	if err != nil {
		return m, fmt.Errorf("vm_stat: %w", err)
	}
	split, err := parseVMStat(bytes.NewReader(out))
	if err != nil {
		return m, err
	}
	split.Total, split.Pressure, split.SwapUsed, split.SwapTotal = m.Total, m.Pressure, m.SwapUsed, m.SwapTotal
	split.Used = split.App + split.Wired + split.Compressed
	return split, nil
}

// parseVMStat fills App, Wired, Compressed, Cached and Free, split the way Activity Monitor
// does (purgeable pages count as cache, not as app memory), and the paging counters; all in bytes.
func parseVMStat(r io.Reader) (Memory, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Memory{}, fmt.Errorf("read vm_stat: %w", err)
	}
	size := vmStatPageSize.FindSubmatch(raw)
	if size == nil {
		return Memory{}, errors.New("vm_stat: page size missing")
	}
	// digits-only by the regexp; an overflow reads as the maximum.
	pageSize, _ := strconv.ParseUint(string(size[1]), 10, 64)
	pages := map[string]uint64{}
	for _, m := range vmStatLine.FindAllSubmatch(raw, -1) {
		pages[string(m[1])], _ = strconv.ParseUint(string(m[2]), 10, 64)
	}
	var errs []error
	count := func(key string) uint64 {
		v, ok := pages[key]
		if !ok {
			errs = append(errs, fmt.Errorf("vm_stat key %q missing", key))
		}
		return v
	}
	anonymous, purgeable := count("Anonymous pages"), count("Pages purgeable")
	m := Memory{
		App:        (anonymous - min(anonymous, purgeable)) * pageSize,
		Wired:      count("Pages wired down") * pageSize,
		Compressed: count("Pages occupied by compressor") * pageSize,
		Cached:     (count("File-backed pages") + purgeable) * pageSize,
		Free:       count("Pages free") * pageSize,
		// Counters an older vm_stat does not print read 0 rather than fail the split.
		PageIns:        pages["Pageins"] * pageSize,
		PageOuts:       pages["Pageouts"] * pageSize,
		SwapIns:        pages["Swapins"] * pageSize,
		SwapOuts:       pages["Swapouts"] * pageSize,
		Compressions:   pages["Compressions"] * pageSize,
		Decompressions: pages["Decompressions"] * pageSize,
	}
	if len(errs) > 0 {
		return Memory{}, errors.Join(errs...)
	}
	return m, nil
}
