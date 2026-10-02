// Package alerts turns samples into alerts: a rule has to hold for a while before it
// fires and to stay quiet for a while before it closes.
package alerts

import (
	"math"
	"slices"
	"time"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
)

const (
	KindCPU       = "cpu"
	KindMemory    = "memory"
	KindDisk      = "disk"
	KindTemp      = "temp"
	KindAppCPU    = "app_cpu"
	KindAppMemory = "app_memory"
	KindThermal   = "thermal"
	// KindBatteryLow is the Mac's own battery, KindBTBattery a Bluetooth device's.
	KindBatteryLow = "battery_low"
	KindBTBattery  = "bt_battery"
	KindMemoryUsed = "memory_used"
	KindSwap       = "swap"
	// KindAppRule is one of the user's own rules, settings.Rule.
	KindAppRule = "app_rule"
)

// The system-wide thresholds come from settings; the per-app ones are fixed here.
const (
	// thermalSerious is the first thermal state in which macOS throttles the machine.
	thermalSerious = 2
	// appCPUPercent is percent of one core: a build or a video call sits near 100 and is
	// not a runaway app.
	appCPUPercent = 150
	appCPUHold    = 5 * time.Minute
	// An app alerts when its RSS is above appMemoryMin and more than appMemoryFactor
	// times its lowest reading of the last appMemoryWindow.
	appMemoryMin    = 1 << 30
	appMemoryFactor = 2
	appMemoryWindow = 30 * time.Minute
	// Growth right after launch is an app loading, not a leak.
	appWarmUp = 5 * time.Minute
	// btBatteryPercent is where macOS itself starts warning about an accessory.
	btBatteryPercent = 15
)

// finding is a rule that holds in one sample, before any timing is applied.
type finding struct {
	kind string
	app  string
	// metric tells two rules for one app apart; "" for every kind but KindAppRule.
	metric string
	// hold is how long the rule must hold before it alerts; 0 is the alert_hold setting.
	hold time.Duration
	// limit is the threshold in the unit of the alert's Detail, 0 for a rule without a number;
	// below says the rule holds under it.
	limit float64
	below bool
	// params fill the {placeholders} of the dictionary strings alert.<kind>.title and .detail.
	params map[string]any
}

// rssTrack is an app's lowest RSS since lowAt.
type rssTrack struct {
	low       uint64
	lowAt     time.Time
	warmUntil time.Time
}

// findings applies every rule to one sample. A zero reading (no sensor, failed
// collector) never alerts.
func findings(snap *collector.Snapshot, cfg settings.Settings, rss map[string]rssTrack) []finding {
	var found []finding
	if snap.CPU.Total > float64(cfg.AlertCPU) {
		found = append(found, finding{
			kind:   KindCPU,
			limit:  float64(cfg.AlertCPU),
			params: map[string]any{"value": math.Round(snap.CPU.Total), "limit": cfg.AlertCPU},
		})
	}
	if snap.CPUTemp > float64(cfg.AlertTemp) {
		temp, limit, unit := snap.CPUTemp, float64(cfg.AlertTemp), "°C"
		if cfg.TempUnit == "F" {
			temp, limit, unit = temp*9/5+32, limit*9/5+32, "°F"
		}
		found = append(found, finding{
			kind:   KindTemp,
			limit:  float64(cfg.AlertTemp),
			params: map[string]any{"value": math.Round(temp), "limit": math.Round(limit), "unit": unit},
		})
	}
	if snap.Thermal >= thermalSerious {
		found = append(found, finding{kind: KindThermal})
	}
	if snap.Disk.Total > 0 {
		if free := float64(snap.Disk.Free) / float64(snap.Disk.Total) * 100; free < float64(cfg.AlertDiskFree) {
			found = append(found, finding{
				kind:   KindDisk,
				limit:  float64(cfg.AlertDiskFree),
				below:  true,
				params: map[string]any{"value": tenth(free), "limit": cfg.AlertDiskFree},
			})
		}
	}
	found = append(found, memoryFindings(snap.Memory, cfg)...)
	found = append(found, batteryFindings(snap, cfg)...)
	return append(found, appFindings(snap.Apps, cfg, rss)...)
}

// memoryFindings, batteryFindings and appFindings are findings split for gocyclo.
func memoryFindings(mem collector.Memory, cfg settings.Settings) []finding {
	var found []finding
	if mem.Pressure == collector.PressureCritical {
		found = append(found, finding{
			kind:   KindMemory,
			params: map[string]any{"used": gib(mem.Used), "total": gib(mem.Total)},
		})
	}
	if mem.Total > 0 && cfg.AlertMemory > 0 {
		if used := float64(mem.Used) / float64(mem.Total) * 100; used > float64(cfg.AlertMemory) {
			found = append(found, finding{
				kind:   KindMemoryUsed,
				limit:  float64(cfg.AlertMemory),
				params: map[string]any{"value": math.Round(used), "limit": cfg.AlertMemory},
			})
		}
	}
	if cfg.AlertSwap > 0 && mem.SwapUsed > uint64(cfg.AlertSwap)<<30 {
		found = append(found, finding{
			kind:   KindSwap,
			limit:  float64(uint64(cfg.AlertSwap) << 30),
			params: map[string]any{"value": gib(mem.SwapUsed), "limit": cfg.AlertSwap},
		})
	}
	return found
}

func batteryFindings(snap *collector.Snapshot, cfg settings.Settings) []finding {
	var found []finding
	// On the charger a low level is already being dealt with.
	if b := snap.Battery; cfg.AlertBattery > 0 && b.Present && !b.ExternalConnected && b.Percent <= cfg.AlertBattery {
		found = append(found, finding{
			kind:   KindBatteryLow,
			limit:  float64(cfg.AlertBattery),
			below:  true,
			params: map[string]any{"value": b.Percent, "limit": cfg.AlertBattery},
		})
	}
	for _, d := range snap.Bluetooth {
		lowest := 100
		for _, l := range d.Levels {
			lowest = min(lowest, l.Percent)
		}
		if lowest <= btBatteryPercent {
			found = append(found, finding{
				kind: KindBTBattery, app: d.Name, limit: btBatteryPercent, below: true,
				params: map[string]any{"app": d.Name, "value": lowest},
			})
		}
	}
	return found
}

func appFindings(apps []collector.App, cfg settings.Settings, rss map[string]rssTrack) []finding {
	var found []finding
	for _, app := range apps {
		if muted(cfg, app.Name) {
			continue
		}
		if app.CPU > appCPUPercent {
			found = append(found, finding{
				kind:   KindAppCPU,
				app:    app.Name,
				hold:   appCPUHold,
				limit:  appCPUPercent,
				params: map[string]any{"app": app.Name, "value": math.Round(app.CPU), "minutes": appCPUHold.Minutes()},
			})
		}
		if track, ok := rss[app.Name]; ok && app.RSS > appMemoryMin && app.RSS > appMemoryFactor*track.low {
			found = append(found, finding{
				kind:  KindAppMemory,
				app:   app.Name,
				limit: float64(max(appMemoryMin, appMemoryFactor*track.low)),
				params: map[string]any{
					"app": app.Name, "value": gib(app.RSS), "low": gib(track.low), "minutes": appMemoryWindow.Minutes(),
				},
			})
		}
		// Two rules with one app and metric share an alert, the later one's numbers win.
		for _, r := range cfg.AlertRules {
			if collector.CleanText(r.App, 0) != collector.CleanText(app.Name, 0) {
				continue
			}
			value, unit, limit := math.Round(app.CPU), "%", r.Limit
			over := app.CPU > r.Limit
			if r.Metric == "memory" {
				limit = r.Limit * (1 << 30)
				value, unit, over = gib(app.RSS), "GB", float64(app.RSS) > limit
			}
			if !over {
				continue
			}
			found = append(found, finding{
				kind: KindAppRule, app: app.Name, metric: r.Metric, hold: time.Duration(r.Minutes) * time.Minute, limit: limit,
				params: map[string]any{
					"app": app.Name, "metric": r.Metric, "value": value, "limit": r.Limit, "unit": unit, "minutes": r.Minutes,
				},
			})
		}
	}
	return found
}

// muted compares names as the page shows them: that is where the muted list comes from.
func muted(cfg settings.Settings, app string) bool {
	app = collector.CleanText(app, 0)
	return slices.ContainsFunc(cfg.AlertMuted, func(name string) bool { return collector.CleanText(name, 0) == app })
}

func tenth(v float64) float64 {
	return math.Round(v*10) / 10
}

func gib(bytes uint64) float64 {
	return tenth(float64(bytes) / (1 << 30))
}
