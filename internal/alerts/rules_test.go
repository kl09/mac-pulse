package alerts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
)

//nolint:funlen // one table for the function under test; the length is the cases.
func TestFindings(t *testing.T) {
	t.Parallel()

	const gib = 1 << 30

	tests := []struct {
		name string
		snap collector.Snapshot
		cfg  func(*settings.Settings)
		rss  map[string]rssTrack
		want []finding
	}{
		{
			name: "quiet sample",
			snap: collector.Snapshot{
				CPU: collector.CPU{Total: 12}, CPUTemp: 55, Disk: collector.Disk{Total: 1000, Free: 500},
				Memory: collector.Memory{Pressure: collector.PressureNormal},
				Apps:   []collector.App{{Name: "Safari", CPU: 20, RSS: gib / 2}},
			},
			rss: map[string]rssTrack{"Safari": {low: gib / 2}},
		},
		{name: "empty sample from failed collectors"},
		{
			name: "cpu above the threshold",
			snap: collector.Snapshot{CPU: collector.CPU{Total: 93.4}},
			want: []finding{{kind: KindCPU, limit: 90, params: map[string]any{"value": 93.0, "limit": 90}}},
		},
		{name: "cpu at the threshold", snap: collector.Snapshot{CPU: collector.CPU{Total: 90}}},
		{
			name: "cpu threshold comes from settings",
			snap: collector.Snapshot{CPU: collector.CPU{Total: 60}},
			cfg:  func(s *settings.Settings) { s.AlertCPU = 50 },
			want: []finding{{kind: KindCPU, limit: 50, params: map[string]any{"value": 60.0, "limit": 50}}},
		},
		{
			name: "temperature above the threshold",
			snap: collector.Snapshot{CPUTemp: 101},
			want: []finding{{kind: KindTemp, limit: 95, params: map[string]any{"value": 101.0, "limit": 95.0, "unit": "°C"}}},
		},
		{
			name: "temperature detail follows the unit setting",
			snap: collector.Snapshot{CPUTemp: 100},
			cfg:  func(s *settings.Settings) { s.TempUnit = "F" },
			want: []finding{{kind: KindTemp, limit: 95, params: map[string]any{"value": 212.0, "limit": 203.0, "unit": "°F"}}},
		},
		{name: "fair thermal state", snap: collector.Snapshot{Thermal: 1}},
		{
			name: "serious thermal state",
			snap: collector.Snapshot{Thermal: 2},
			want: []finding{{kind: KindThermal}},
		},
		{
			name: "critical thermal state",
			snap: collector.Snapshot{Thermal: 3},
			want: []finding{{kind: KindThermal}},
		},
		{
			name: "disk free below the threshold",
			snap: collector.Snapshot{Disk: collector.Disk{Total: 1000, Free: 42}},
			want: []finding{{kind: KindDisk, limit: 10, below: true, params: map[string]any{"value": 4.2, "limit": 10}}},
		},
		{name: "disk free at the threshold", snap: collector.Snapshot{Disk: collector.Disk{Total: 1000, Free: 100}}},
		{
			name: "disk threshold comes from settings",
			snap: collector.Snapshot{Disk: collector.Disk{Total: 1000, Free: 42}},
			cfg:  func(s *settings.Settings) { s.AlertDiskFree = 4 },
		},
		{
			name: "critical memory pressure",
			snap: collector.Snapshot{Memory: collector.Memory{Pressure: collector.PressureCritical, Used: 23 * gib, Total: 24 * gib}},
			want: []finding{{kind: KindMemory, params: map[string]any{"used": 23.0, "total": 24.0}}},
		},
		{name: "warning memory pressure", snap: collector.Snapshot{Memory: collector.Memory{Pressure: collector.PressureWarning}}},
		{
			name: "every app above 150% of a core",
			snap: collector.Snapshot{Apps: []collector.App{
				{Name: "yes", CPU: 99.6}, {Name: "xz", CPU: 380}, {Name: "edge", CPU: 150}, {Name: "ffmpeg", CPU: 150.4},
			}},
			want: []finding{
				{
					kind: KindAppCPU, app: "xz", hold: appCPUHold, limit: appCPUPercent,
					params: map[string]any{"app": "xz", "value": 380.0, "minutes": 5.0},
				},
				{
					kind: KindAppCPU, app: "ffmpeg", hold: appCPUHold, limit: appCPUPercent,
					params: map[string]any{"app": "ffmpeg", "value": 150.0, "minutes": 5.0},
				},
			},
		},
		{
			name: "app memory more than doubled and above 1 GB",
			snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 3 * gib}}},
			rss:  map[string]rssTrack{"Chrome": {low: gib + gib/4}},
			want: []finding{
				{
					kind: KindAppMemory, app: "Chrome", limit: 2 * (gib + gib/4),
					params: map[string]any{"app": "Chrome", "value": 3.0, "low": 1.3, "minutes": 30.0},
				},
			},
		},
		{
			name: "app memory exactly doubled",
			snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 3 * gib}}},
			rss:  map[string]rssTrack{"Chrome": {low: 3 * gib / 2}},
		},
		{
			name: "app memory doubled but not above 1 GB",
			snap: collector.Snapshot{Apps: []collector.App{{Name: "Notes", RSS: gib}}},
			rss:  map[string]rssTrack{"Notes": {low: gib / 10}},
		},
		{
			name: "app without a tracked low",
			snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 3 * gib}}},
		},
		{
			name: "battery at the threshold while on battery",
			snap: collector.Snapshot{Battery: collector.Battery{Present: true, Percent: 20}},
			want: []finding{{kind: KindBatteryLow, limit: 20, below: true, params: map[string]any{"value": 20, "limit": 20}}},
		},
		{name: "battery above the threshold", snap: collector.Snapshot{Battery: collector.Battery{Present: true, Percent: 21}}},
		{
			name: "low battery on the charger",
			snap: collector.Snapshot{Battery: collector.Battery{Present: true, Percent: 5, ExternalConnected: true}},
		},
		{
			name: "battery alert switched off",
			snap: collector.Snapshot{Battery: collector.Battery{Present: true}},
			cfg:  func(s *settings.Settings) { s.AlertBattery = 0 },
		},
		{
			name: "bluetooth device alerts on its lowest part, by name",
			snap: collector.Snapshot{Bluetooth: []collector.BluetoothDevice{
				{Name: "AirPods", Levels: []collector.BatteryLevel{{Part: "left", Percent: 60}, {Part: "right", Percent: 9}}},
				{Name: "Mouse", Levels: []collector.BatteryLevel{{Part: "main", Percent: 15}}},
				{Name: "Keyboard", Levels: []collector.BatteryLevel{{Part: "main", Percent: 16}}},
			}},
			want: []finding{
				{kind: KindBTBattery, app: "AirPods", limit: btBatteryPercent, below: true, params: map[string]any{"app": "AirPods", "value": 9}},
				{kind: KindBTBattery, app: "Mouse", limit: btBatteryPercent, below: true, params: map[string]any{"app": "Mouse", "value": 15}},
			},
		},
		{
			name: "memory used above its threshold",
			snap: collector.Snapshot{Memory: collector.Memory{Used: 916, Total: 1000}},
			cfg:  func(s *settings.Settings) { s.AlertMemory = 90 },
			want: []finding{{kind: KindMemoryUsed, limit: 90, params: map[string]any{"value": 92.0, "limit": 90}}},
		},
		{
			name: "memory used at its threshold",
			snap: collector.Snapshot{Memory: collector.Memory{Used: 900, Total: 1000}},
			cfg:  func(s *settings.Settings) { s.AlertMemory = 90 },
		},
		{name: "memory used alert is off by default", snap: collector.Snapshot{Memory: collector.Memory{Used: 999, Total: 1000}}},
		{
			name: "swap above its threshold",
			snap: collector.Snapshot{Memory: collector.Memory{SwapUsed: 5*gib + gib/4}},
			cfg:  func(s *settings.Settings) { s.AlertSwap = 4 },
			want: []finding{{kind: KindSwap, limit: 4 * gib, params: map[string]any{"value": 5.3, "limit": 4}}},
		},
		{
			name: "swap at its threshold",
			snap: collector.Snapshot{Memory: collector.Memory{SwapUsed: 4 * gib}},
			cfg:  func(s *settings.Settings) { s.AlertSwap = 4 },
		},
		{name: "swap alert is off by default", snap: collector.Snapshot{Memory: collector.Memory{SwapUsed: 60 * gib}}},
		{
			name: "own rules: cpu and memory of one app are two findings, another app's rule stays quiet",
			snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", CPU: 80.4, RSS: 9 * gib}, {Name: "Slack", CPU: 80, RSS: gib}}},
			cfg: func(s *settings.Settings) {
				s.AlertRules = []settings.Rule{
					{App: "Xcode", Metric: "cpu", Limit: 50, Minutes: 2},
					{App: "Xcode", Metric: "memory", Limit: 8.5, Minutes: 10},
					{App: "Slack", Metric: "cpu", Limit: 80, Minutes: 1},
					{App: "Safari", Metric: "cpu", Limit: 1, Minutes: 1},
				}
			},
			want: []finding{
				{
					kind: KindAppRule, app: "Xcode", metric: "cpu", hold: 2 * time.Minute, limit: 50,
					params: map[string]any{"app": "Xcode", "metric": "cpu", "value": 80.0, "limit": 50.0, "unit": "%", "minutes": 2},
				},
				{
					kind: KindAppRule, app: "Xcode", metric: "memory", hold: 10 * time.Minute, limit: 8.5 * gib,
					params: map[string]any{"app": "Xcode", "metric": "memory", "value": 9.0, "limit": 8.5, "unit": "GB", "minutes": 10},
				},
			},
		},
		{
			name: "muted app raises no per-app alert of any kind, the others still do",
			snap: collector.Snapshot{
				Apps:      []collector.App{{Name: "xz", CPU: 380, RSS: 3 * gib}, {Name: "yes", CPU: 200}},
				Bluetooth: []collector.BluetoothDevice{{Name: "xz", Levels: []collector.BatteryLevel{{Part: "main", Percent: 3}}}},
			},
			cfg: func(s *settings.Settings) {
				s.AlertMuted = []string{"xz"}
				s.AlertRules = []settings.Rule{{App: "xz", Metric: "cpu", Limit: 10, Minutes: 1}}
			},
			rss: map[string]rssTrack{"xz": {low: gib}},
			want: []finding{
				{kind: KindBTBattery, app: "xz", limit: btBatteryPercent, below: true, params: map[string]any{"app": "xz", "value": 3}},
				{
					kind: KindAppCPU, app: "yes", hold: appCPUHold, limit: appCPUPercent,
					params: map[string]any{"app": "yes", "value": 200.0, "minutes": 5.0},
				},
			},
		},
		{
			name: "names match as the page shows them: a bidi control in the app name or in the muted list does not escape a mute or a rule",
			snap: collector.Snapshot{Apps: []collector.App{{Name: "x\u202ez", CPU: 380}, {Name: "ab", CPU: 380}, {Name: "ye\u202es", CPU: 20}}},
			cfg: func(s *settings.Settings) {
				s.AlertMuted = []string{"xz", "a\u202eb"}
				s.AlertRules = []settings.Rule{{App: "yes", Metric: "cpu", Limit: 10, Minutes: 1}}
			},
			want: []finding{{
				kind: KindAppRule, app: "ye\u202es", metric: "cpu", hold: time.Minute, limit: 10,
				params: map[string]any{"app": "ye\u202es", "metric": "cpu", "value": 20.0, "limit": 10.0, "unit": "%", "minutes": 1},
			}},
		},
		{
			name: "several rules at once keep a fixed order",
			snap: collector.Snapshot{
				CPU: collector.CPU{Total: 95}, CPUTemp: 99, Disk: collector.Disk{Total: 1000, Free: 10},
				Apps: []collector.App{{Name: "yes", CPU: 200, RSS: 2 * gib}},
			},
			rss: map[string]rssTrack{"yes": {low: gib / 2}},
			want: []finding{
				{kind: KindCPU, limit: 90, params: map[string]any{"value": 95.0, "limit": 90}},
				{kind: KindTemp, limit: 95, params: map[string]any{"value": 99.0, "limit": 95.0, "unit": "°C"}},
				{kind: KindDisk, limit: 10, below: true, params: map[string]any{"value": 1.0, "limit": 10}},
				{
					kind: KindAppCPU, app: "yes", hold: appCPUHold, limit: appCPUPercent,
					params: map[string]any{"app": "yes", "value": 200.0, "minutes": 5.0},
				},
				{kind: KindAppMemory, app: "yes", limit: gib, params: map[string]any{"app": "yes", "value": 2.0, "low": 0.5, "minutes": 30.0}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Default()
			if tt.cfg != nil {
				tt.cfg(&cfg)
			}

			assert.Equal(t, tt.want, findings(&tt.snap, cfg, tt.rss))
		})
	}
}
