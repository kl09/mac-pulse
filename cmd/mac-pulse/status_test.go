package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
	"github.com/kl09/mac-pulse/internal/shell"
	"github.com/kl09/mac-pulse/internal/store"
)

//nolint:funlen // one table of whole-title expectations
func TestStatusItems(t *testing.T) {
	t.Parallel()

	snap := collector.Snapshot{
		CPU:     collector.CPU{Total: 7.4},
		CPUTemp: 63.4,
		Memory:  collector.Memory{Total: 1000, Used: 615},
		Network: collector.Network{DownRate: 1.5 * 1024 * 1024, UpRate: 200 * 1024},
		Battery: collector.Battery{Present: true, Percent: 78},
	}
	spark := store.Spark{
		CPU: []float64{-5, 50, 100, 140}, Memory: []float64{62}, Temp: []float64{20, 65, 120},
		NetDown: []float64{0, 4 << 20}, NetUp: []float64{1 << 20, 0},
	}
	tests := []struct {
		name  string
		snap  collector.Snapshot
		spark store.Spark
		cfg   settings.Settings
		alert bool
		want  []shell.StatusItem
	}{
		{
			name: "default items, not padded", snap: snap, cfg: settings.Default(),
			want: []shell.StatusItem{{Symbol: "cpu", Text: "7%"}, {Symbol: "memorychip", Text: "62%"}},
		},
		{
			name: "items follow the configured order", snap: snap, cfg: settings.Settings{MenuBar: []string{"battery", "temp", "net"}},
			want: []shell.StatusItem{
				{Symbol: "battery.75percent", Text: "78%"},
				{Symbol: "thermometer.medium", Text: "63°"},
				{Symbol: "arrow.down", Text: "1.5 MB/s"},
				{Symbol: "arrow.up", Text: "200 KB/s"},
			},
		},
		{
			name: "fahrenheit and bits", snap: snap, cfg: settings.Settings{MenuBar: []string{"temp", "net"}, TempUnit: "F", NetUnit: "bits"},
			want: []shell.StatusItem{
				{Symbol: "thermometer.medium", Text: "146°"}, {Symbol: "arrow.down", Text: "13 Mb/s"}, {Symbol: "arrow.up", Text: "1.6 Mb/s"},
			},
		},
		{
			name: "an idle link shows kilobytes, never bytes",
			snap: collector.Snapshot{Network: collector.Network{DownRate: 300}}, cfg: settings.Settings{MenuBar: []string{"net"}},
			want: []shell.StatusItem{{Symbol: "arrow.down", Text: "0 KB/s"}, {Symbol: "arrow.up", Text: "0 KB/s"}},
		},
		{
			name: "a rate that would round to four digits moves to the next unit",
			snap: collector.Snapshot{Network: collector.Network{DownRate: 999.8 * 1024}}, cfg: settings.Settings{MenuBar: []string{"net"}},
			want: []shell.StatusItem{{Symbol: "arrow.down", Text: "1.0 MB/s"}, {Symbol: "arrow.up", Text: "0 KB/s"}},
		},
		{
			name: "full CPU and full battery", cfg: settings.Settings{MenuBar: []string{"cpu", "battery"}},
			snap: collector.Snapshot{CPU: collector.CPU{Total: 100}, Battery: collector.Battery{Present: true, Percent: 100}},
			want: []shell.StatusItem{{Symbol: "cpu", Text: "100%"}, {Symbol: "battery.100percent", Text: "100%"}},
		},
		{
			name: "no sensor, no battery and no memory reading", cfg: settings.Settings{MenuBar: []string{"temp", "battery", "mem"}},
			want: []shell.StatusItem{
				{Symbol: "thermometer.medium", Text: "—"},
				{Symbol: "battery.0percent", Text: "—"},
				{Symbol: "memorychip", Text: "0%"},
			},
		},
		{
			name: "an active alert puts a warning first", snap: snap, cfg: settings.Settings{MenuBar: []string{"cpu"}}, alert: true,
			want: []shell.StatusItem{{Symbol: "exclamationmark.triangle.fill", Warn: true}, {Symbol: "cpu", Text: "7%"}},
		},
		{name: "an unknown item is skipped", snap: snap, cfg: settings.Settings{MenuBar: []string{"fans"}}},
		{
			name: "disk shows the free share; no total, no number", cfg: settings.Settings{MenuBar: []string{"disk", "disk"}},
			snap: collector.Snapshot{Disk: collector.Disk{Total: 1000, Free: 254}},
			want: []shell.StatusItem{{Symbol: "internaldrive", Text: "25%"}, {Symbol: "internaldrive", Text: "25%"}},
		},
		{
			name: "disk before the first reading", cfg: settings.Settings{MenuBar: []string{"disk"}},
			want: []shell.StatusItem{{Symbol: "internaldrive", Text: "—"}},
		},
		{
			name: "sparklines stay out with the graph setting off", snap: snap, spark: spark,
			cfg: settings.Settings{MenuBar: []string{"cpu"}}, want: []shell.StatusItem{{Symbol: "cpu", Text: "7%"}},
		},
		{
			name: "graphs: percent as is, temperature over its span, both rates on one scale, none for battery and disk",
			snap: snap, spark: spark,
			cfg: settings.Settings{MenuBar: []string{"cpu", "mem", "temp", "net", "battery", "disk"}, MenuBarGraph: true},
			want: []shell.StatusItem{
				{Symbol: "cpu", Text: "7%", Points: []float64{0, 0.5, 1, 1}},
				{Symbol: "memorychip", Text: "62%", Points: []float64{0.62}},
				{Symbol: "thermometer.medium", Text: "63°", Points: []float64{0, 0.5, 1}},
				{Symbol: "arrow.down", Text: "1.5 MB/s", Points: []float64{0, 1}},
				{Symbol: "arrow.up", Text: "200 KB/s", Points: []float64{0.25, 0}},
				{Symbol: "battery.75percent", Text: "78%"},
				{Symbol: "internaldrive", Text: "—"},
			},
		},
		{
			name: "graphs of an idle link stay flat", snap: collector.Snapshot{}, spark: store.Spark{NetDown: []float64{640}, NetUp: []float64{0}},
			cfg: settings.Settings{MenuBar: []string{"net"}, MenuBarGraph: true},
			want: []shell.StatusItem{
				{Symbol: "arrow.down", Text: "0 KB/s", Points: []float64{0.01}}, {Symbol: "arrow.up", Text: "0 KB/s", Points: []float64{0}},
			},
		},
		{
			name: "graph setting on before the first sample", snap: snap,
			cfg:  settings.Settings{MenuBar: []string{"cpu", "net"}, MenuBarGraph: true},
			want: []shell.StatusItem{{Symbol: "cpu", Text: "7%"}, {Symbol: "arrow.down", Text: "1.5 MB/s"}, {Symbol: "arrow.up", Text: "200 KB/s"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, statusItems(&tt.snap, tt.spark, tt.cfg, tt.alert))
		})
	}

	// The shell draws what this JSON says: a new key in it would change the frozen default look.
	t.Run("default JSON", func(t *testing.T) {
		t.Parallel()

		got, err := json.Marshal(statusItems(&snap, spark, settings.Default(), false))

		require.NoError(t, err)
		assert.JSONEq(t, `[{"symbol":"cpu","text":"7%"},{"symbol":"memorychip","text":"62%"}]`, string(got))
	})
}

func TestStatusGroups(t *testing.T) {
	t.Parallel()

	snap := collector.Snapshot{CPU: collector.CPU{Total: 7.4}, Memory: collector.Memory{Total: 1000, Used: 615}}
	tests := []struct {
		name    string
		menuBar []string
		alert   bool
		want    [][]shell.StatusItem
	}{
		{
			name: "one group per entry, in the configured order", menuBar: []string{"mem", "cpu"},
			want: [][]shell.StatusItem{{{Symbol: "memorychip", Text: "62%"}}, {{Symbol: "cpu", Text: "7%"}}},
		},
		{
			name: "the warning triangle leads the first group only", menuBar: []string{"cpu", "mem"}, alert: true,
			want: [][]shell.StatusItem{
				{{Symbol: "exclamationmark.triangle.fill", Warn: true}, {Symbol: "cpu", Text: "7%"}},
				{{Symbol: "memorychip", Text: "62%"}},
			},
		},
		{
			name: "both directions of the network stay in one group", menuBar: []string{"net"},
			want: [][]shell.StatusItem{{{Symbol: "arrow.down", Text: "0 KB/s"}, {Symbol: "arrow.up", Text: "0 KB/s"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := statusGroups(&snap, store.Spark{}, settings.Settings{MenuBar: tt.menuBar}, tt.alert)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClockTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  settings.Settings
		want string
	}{
		{name: "off by default: no clock item", cfg: settings.Default()},
		{name: "the language picks between 12 and 24 hours", cfg: settings.Settings{Clock: true, ClockHours: "auto"}, want: "jmm"},
		{name: "24 hours", cfg: settings.Settings{Clock: true, ClockHours: "24"}, want: "HHmm"},
		{name: "12 hours with seconds", cfg: settings.Settings{Clock: true, ClockHours: "12", ClockSeconds: true}, want: "hmmass"},
		{
			name: "date and seconds", cfg: settings.Settings{Clock: true, ClockHours: "24", ClockDate: true, ClockSeconds: true},
			want: "EEEdMMMHHmmss",
		},
		{name: "the parts without the clock itself show nothing", cfg: settings.Settings{ClockDate: true, ClockSeconds: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, clockTemplate(tt.cfg))
		})
	}
}
