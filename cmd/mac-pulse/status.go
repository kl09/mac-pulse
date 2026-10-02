package main

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/shell"
	"github.com/kl09/mac-pulse/internal/store"
	"github.com/kl09/mac-pulse/internal/ui"
)

// statusKeys are the menu_bar entries, and statusTabs the screen a click on the separate
// status item of each opens.
var (
	statusKeys = []string{"cpu", "mem", "net", "temp", "battery", "disk"}
	statusTabs = map[string]string{
		"cpu": ui.TabCPU, "mem": ui.TabMemory, "net": ui.TabNetInfo, "temp": ui.TabSensors, "battery": ui.TabBattery, "disk": ui.TabDisk,
	}
)

// The menu bar graph of the temperature spans what a running Mac reads, in °C.
const (
	graphTempMin = 30.0
	graphTempMax = 100.0
	// A link that moves less than this draws a flat line, not its noise at full height.
	graphRateMin = 64 * 1024.0
)

// fixedSample is what the status item shows under MAC_PULSE_STATUS_FIXED, so that its
// snapshot can be compared byte for byte with a reference.
var fixedSample = collector.Snapshot{
	CPU:     collector.CPU{Total: 37},
	CPUTemp: 63,
	Memory:  collector.Memory{Total: 100, Used: 62},
	Network: collector.Network{DownRate: 1.5 * 1024 * 1024, UpRate: 200 * 1024},
	Battery: collector.Battery{Present: true, Percent: 78},
	Disk:    collector.Disk{Total: 100, Free: 25},
}

// statusItems renders the menu bar title: the items the user picked, in their order,
// behind a warning triangle while an alert is active. With the menu_bar_graph setting on,
// cpu, mem, net and temp carry their sparkline; without it no item has Points.
func statusItems(snap *collector.Snapshot, spark store.Spark, cfg settings.Settings, alert bool) []shell.StatusItem {
	var items []shell.StatusItem
	if alert {
		items = append(items, shell.StatusItem{Symbol: "exclamationmark.triangle.fill", Warn: true})
	}
	if !cfg.MenuBarGraph {
		spark = store.Spark{}
	}
	for _, key := range cfg.MenuBar {
		switch key {
		case "cpu":
			items = append(items, shell.StatusItem{Symbol: "cpu", Text: fmt.Sprintf("%.0f%%", snap.CPU.Total), Points: graph(spark.CPU, 0, 100)})
		case "mem":
			var used float64
			if snap.Memory.Total > 0 {
				used = float64(snap.Memory.Used) / float64(snap.Memory.Total) * 100
			}
			items = append(items, shell.StatusItem{Symbol: "memorychip", Text: fmt.Sprintf("%.0f%%", used), Points: graph(spark.Memory, 0, 100)})
		case "net":
			// One scale for both directions, so an idle upload stays flat next to a download.
			top := slices.Max(slices.Concat([]float64{graphRateMin}, spark.NetDown, spark.NetUp))
			items = append(items,
				shell.StatusItem{Symbol: "arrow.down", Text: rateText(snap.Network.DownRate, cfg.NetUnit), Points: graph(spark.NetDown, 0, top)},
				shell.StatusItem{Symbol: "arrow.up", Text: rateText(snap.Network.UpRate, cfg.NetUnit), Points: graph(spark.NetUp, 0, top)})
		case "temp":
			text := "—"
			switch {
			case snap.CPUTemp <= 0:
			case cfg.TempUnit == "F":
				text = fmt.Sprintf("%.0f°", snap.CPUTemp*9/5+32)
			default:
				text = fmt.Sprintf("%.0f°", snap.CPUTemp)
			}
			items = append(items, shell.StatusItem{Symbol: "thermometer.medium", Text: text, Points: graph(spark.Temp, graphTempMin, graphTempMax)})
		case "battery":
			item := shell.StatusItem{Symbol: "battery.0percent", Text: "—"}
			if b := snap.Battery; b.Present {
				// SF Symbols draws the battery in quarter steps.
				item.Symbol = fmt.Sprintf("battery.%dpercent", (b.Percent+12)/25*25)
				item.Text = fmt.Sprintf("%d%%", b.Percent)
			}
			items = append(items, item)
		case "disk":
			// Free space of the boot volume, as on the Disk tile.
			text := "—"
			if snap.Disk.Total > 0 {
				text = fmt.Sprintf("%.0f%%", float64(snap.Disk.Free)/float64(snap.Disk.Total)*100)
			}
			items = append(items, shell.StatusItem{Symbol: "internaldrive", Text: text})
		}
	}
	return items
}

// statusGroups is the title cut into one group per menu_bar entry, in their order, for the
// separate status items; the warning triangle leads the first group.
func statusGroups(snap *collector.Snapshot, spark store.Spark, cfg settings.Settings, alert bool) [][]shell.StatusItem {
	groups := make([][]shell.StatusItem, 0, len(cfg.MenuBar))
	for i, key := range cfg.MenuBar {
		one := cfg
		one.MenuBar = []string{key}
		groups = append(groups, statusItems(snap, spark, one, alert && i == 0))
	}
	return groups
}

// clockTemplate is the NSDateFormatter template of the menu bar clock, "" while the clock is
// off. The shell's formatter orders and names the parts by the interface language; "j" is
// that language's own choice between the 12 and the 24 hour clock.
func clockTemplate(cfg settings.Settings) string {
	if !cfg.Clock {
		return ""
	}
	template := cmp.Or(map[string]string{"12": "hmma", "24": "HHmm"}[cfg.ClockHours], "jmm")
	if cfg.ClockSeconds {
		template += "ss"
	}
	if cfg.ClockDate {
		template = "EEEdMMM" + template
	}
	return template
}

// graph scales a sparkline to the 0–1 the shell draws; nil for no points, so the item
// marshals without the key.
func graph(points []float64, low, high float64) []float64 {
	if len(points) == 0 {
		return nil
	}
	out := make([]float64, len(points))
	for i, v := range points {
		// Two decimals are a third of a point on a 24 pt graph.
		out[i] = math.Round(min(max((v-low)/(high-low), 0), 1)*100) / 100
	}
	return out
}

// rateText is a number of at most three characters, a thin space and a unit. It
// starts at kilo: bytes per second would flicker through every width on an idle link.
func rateText(bytesPerSecond float64, unit string) string {
	v, base, units := bytesPerSecond, 1024.0, []string{"KB/s", "MB/s", "GB/s"}
	if unit == "bits" {
		v, base, units = v*8, 1000, []string{"kb/s", "Mb/s", "Gb/s"}
	}
	v /= base
	i := 0
	for v >= 999.5 && i < len(units)-1 {
		v /= base
		i++
	}
	number := fmt.Sprintf("%.0f", v)
	if i > 0 && v < 9.95 {
		number = fmt.Sprintf("%.1f", v)
	}
	return number + " " + units[i]
}
