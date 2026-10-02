package ui

import (
	"fmt"
	"strings"
	"time"
)

// summaryApps is how many of the busiest apps the copied text names.
const summaryApps = 3

// Summary is the plain text behind "Copy stats": what the overview shows, one line per
// metric. A source the Mac lacks (battery, die sensor, power) has no line.
func Summary(st State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mac-pulse · %s\n", time.UnixMilli(st.Time).Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "CPU: %.0f%% (user %.0f%%, system %.0f%%), load %.2f %.2f %.2f\n",
		st.CPU.Total, st.CPU.User, st.CPU.System, st.CPU.Load[0], st.CPU.Load[1], st.CPU.Load[2])
	fmt.Fprintf(&b, "Memory: %s of %s used, pressure %s\n",
		byteText(float64(st.Memory.Used), 1024), byteText(float64(st.Memory.Total), 1024), st.Memory.Pressure)
	fmt.Fprintf(&b, "Disk: %s free of %s\n", byteText(float64(st.Disk.Free), 1000), byteText(float64(st.Disk.Total), 1000))
	fmt.Fprintf(&b, "Network: down %s/s, up %s/s\n", byteText(st.Network.DownRate, 1024), byteText(st.Network.UpRate, 1024))
	if bat := st.Battery; bat != nil {
		fmt.Fprintf(&b, "Battery: %d%% (%s), health %d%%, %d cycles\n", bat.Percent, bat.State, bat.Health, bat.Cycles)
	}
	if t := st.CPU.Temp; t != nil {
		// The state carries °C; the text follows the unit the user reads everywhere else.
		if st.Settings.TempUnit == "F" {
			fmt.Fprintf(&b, "Temperature: %.0f °F, thermal state %s\n", *t*9/5+32, st.Sensors.Thermal)
		} else {
			fmt.Fprintf(&b, "Temperature: %.0f °C, thermal state %s\n", *t, st.Sensors.Thermal)
		}
	}
	if w := st.Power.System; w != nil {
		fmt.Fprintf(&b, "Power: %.1f W\n", *w)
	}
	// Items arrive sorted by CPU.
	apps := make([]string, 0, summaryApps)
	for _, a := range st.Apps.Items[:min(len(st.Apps.Items), summaryApps)] {
		apps = append(apps, fmt.Sprintf("%s %.1f%% / %s", a.Name, a.CPU, byteText(float64(a.Memory), 1024)))
	}
	if len(apps) > 0 && st.Apps.HasRates {
		fmt.Fprintf(&b, "Top apps (CPU / memory): %s\n", strings.Join(apps, ", "))
	}
	return b.String()
}

// byteText prints three significant digits at most: "912 KB", "14.2 GB". base is 1024 for
// memory, like Activity Monitor, and 1000 for disk space, like Finder; the panel does the same.
func byteText(v, base float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= 999.5 && i < len(units)-1 {
		v /= base
		i++
	}
	if i > 0 && v < 99.95 {
		return fmt.Sprintf("%.1f %s", v, units[i])
	}
	return fmt.Sprintf("%.0f %s", v, units[i])
}
