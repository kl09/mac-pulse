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
	"strings"
	"time"
)

// Anchored to line start: the nested "BatteryData" = {..."CycleCount"=...} line
// must not match.
var (
	batteryLine  = regexp.MustCompile(`(?m)^\s+"(\w+)" = (.+)$`)
	adapterName  = regexp.MustCompile(`"Name"="([^"]*)"`)
	adapterWatts = regexp.MustCompile(`"Watts"=(\d+)`)
)

func readBattery(ctx context.Context) (Battery, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ioreg", "-r", "-c", "AppleSmartBattery").Output()
	if err != nil {
		return Battery{}, fmt.Errorf("ioreg battery: %w", err)
	}
	return parseBattery(bytes.NewReader(out))
}

func parseBattery(r io.Reader) (Battery, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Battery{}, fmt.Errorf("read ioreg battery: %w", err)
	}
	vals := map[string]string{}
	for _, m := range batteryLine.FindAllStringSubmatch(string(raw), -1) {
		vals[m[1]] = m[2]
	}
	var errs []error
	// ioreg prints negative amperage as a wrapped uint64 (18446744073709551584 = -32).
	num := func(key string) int64 {
		v, ok := vals[key]
		if !ok {
			errs = append(errs, fmt.Errorf("battery key %q missing", key))
			return 0
		}
		u, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("battery key %q: %w", key, err))
		}
		return int64(u)
	}
	// AppleSmartBattery reports 65535 minutes when it has no estimate.
	remaining := num("TimeRemaining")
	if remaining >= 65535 {
		remaining = 0
	}
	b := Battery{
		Present:           true,
		Percent:           int(num("CurrentCapacity")),
		IsCharging:        vals["IsCharging"] == "Yes",
		ExternalConnected: vals["ExternalConnected"] == "Yes",
		TimeRemaining:     time.Duration(remaining) * time.Minute,
		CycleCount:        int(num("CycleCount")),
		Temperature:       float64(num("Temperature")) / 100,
		Voltage:           num("Voltage"),
		Amperage:          num("InstantAmperage"),
	}
	b.DesignMAh = int(num("DesignCapacity"))
	// System Information shows the nominal capacity; the raw one moves with every gauge
	// reading and is the fallback for a battery that has no nominal key.
	if b.MaxMAh, _ = strconv.Atoi(vals["NominalChargeCapacity"]); b.MaxMAh == 0 {
		b.MaxMAh = int(num("AppleRawMaxCapacity"))
	}
	if b.DesignMAh > 0 {
		b.Health = b.MaxMAh * 100 / b.DesignMAh
	}
	// On battery the dictionary stays but loses its Watts.
	if watts := adapterWatts.FindStringSubmatch(vals["AdapterDetails"]); watts != nil && b.ExternalConnected {
		b.Adapter = &Adapter{}
		b.Adapter.Watts, _ = strconv.Atoi(watts[1])
		if name := adapterName.FindStringSubmatch(vals["AdapterDetails"]); name != nil {
			b.Adapter.Name = strings.TrimSpace(name[1])
		}
	}
	b.Power = float64(b.Voltage) * float64(b.Amperage) / 1e6
	if len(errs) > 0 {
		return Battery{}, errors.Join(errs...)
	}
	return b, nil
}
