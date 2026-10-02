package alerts

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
)

//nolint:funlen // one table for the function under test; the length is the cases.
func TestEngine_Check(t *testing.T) {
	t.Parallel()

	const gib = 1 << 30
	t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	type step struct {
		at   time.Duration
		snap collector.Snapshot
		// off switches alerts off in settings for this sample.
		off bool
		// muted is the alert_muted setting for this sample.
		muted []string
	}
	calm := collector.Snapshot{}
	busy := collector.Snapshot{CPU: collector.CPU{Total: 95}}
	hog := collector.Snapshot{
		Apps:      []collector.App{{Name: "yes", CPU: 200}},
		Bluetooth: []collector.BluetoothDevice{{Name: "yes", Levels: []collector.BatteryLevel{{Part: "main", Percent: 3}}}},
	}
	cpuAlert := Alert{
		ID: fmt.Sprintf("cpu::%d", t0.Unix()), Kind: KindCPU, Since: t0,
		Params: map[string]any{"value": 95.0, "limit": 90},
	}
	cpuClosed := cpuAlert
	cpuClosed.Until = t0.Add(time.Minute)
	cpuAgain, cpuNextHour := cpuAlert, cpuAlert
	cpuAgain.ID, cpuAgain.Since = fmt.Sprintf("cpu::%d", t0.Add(10*time.Minute).Unix()), t0.Add(10*time.Minute)
	cpuNextHour.ID, cpuNextHour.Since = fmt.Sprintf("cpu::%d", t0.Add(time.Hour).Unix()), t0.Add(time.Hour)
	// 21 alerts, one every five minutes: open at +0 s, fire at +60 s, close by +100 s.
	cycles := make([]step, 0, 3*(maxRecent+1))
	var lastTwenty []Alert
	for i := range maxRecent + 1 {
		start := time.Duration(i) * 5 * time.Minute
		cycles = append(cycles,
			step{at: start, snap: busy}, step{at: start + time.Minute, snap: busy}, step{at: start + 100*time.Second, snap: calm})
		if i > 0 {
			closed := cpuAlert
			closed.ID, closed.Since, closed.Until = fmt.Sprintf("cpu::%d", t0.Add(start).Unix()), t0.Add(start), t0.Add(start+time.Minute)
			lastTwenty = append([]Alert{closed}, lastTwenty...)
		}
	}

	tests := []struct {
		name  string
		steps []step
		// cfg changes the default settings of every step.
		cfg          func(*settings.Settings)
		wantActive   []Alert
		wantRecent   []Alert
		wantNotified []string
	}{
		{name: "calm samples", steps: []step{{at: 0, snap: calm}, {at: 2 * time.Second, snap: calm}}},
		{
			name:  "rule held for under a minute is not an alert yet",
			steps: []step{{at: 0, snap: busy}, {at: 58 * time.Second, snap: busy}},
		},
		{
			name:         "rule held for a minute fires and notifies once",
			steps:        []step{{at: 0, snap: busy}, {at: time.Minute, snap: busy}, {at: 62 * time.Second, snap: busy}},
			wantActive:   []Alert{cpuAlert},
			wantNotified: []string{"cpu "},
		},
		{
			name:         "hold comes from settings: 10 s fires on the second sample",
			steps:        []step{{at: 0, snap: busy}, {at: 10 * time.Second, snap: busy}},
			cfg:          func(s *settings.Settings) { s.AlertHold = 10 },
			wantActive:   []Alert{cpuAlert},
			wantNotified: []string{"cpu "},
		},
		{
			name:  "hold raised in settings keeps a minute-old rule quiet",
			steps: []step{{at: 0, snap: busy}, {at: time.Minute, snap: busy}},
			cfg:   func(s *settings.Settings) { s.AlertHold = 61 },
		},
		{
			name: "own rule holds for its minutes, and two rules of one app are two alerts",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", CPU: 80, RSS: 9 * gib}}}},
				{at: 119 * time.Second, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", CPU: 80, RSS: 9 * gib}}}},
				{at: 2 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", CPU: 70, RSS: 9 * gib}}}},
			},
			cfg: func(s *settings.Settings) {
				s.AlertRules = []settings.Rule{
					{App: "Xcode", Metric: "cpu", Limit: 50, Minutes: 2}, {App: "Xcode", Metric: "memory", Limit: 8, Minutes: 3},
				}
			},
			wantActive: []Alert{{
				ID: fmt.Sprintf("app_rulecpu:Xcode:%d", t0.Unix()), Kind: KindAppRule, App: "Xcode", Since: t0,
				Params: map[string]any{"app": "Xcode", "metric": "cpu", "value": 70.0, "limit": 50.0, "unit": "%", "minutes": 2},
			}},
			wantNotified: []string{"app_rule Xcode"},
		},
		{
			name: "a dip before the alert restarts the hold",
			steps: []step{
				{at: 0, snap: busy},
				{at: 30 * time.Second, snap: busy},
				{at: 40 * time.Second, snap: calm},
				{at: 50 * time.Second, snap: busy},
				{at: 100 * time.Second, snap: busy},
			},
		},
		{
			name: "active alert survives a dip shorter than 30 s",
			steps: []step{
				{at: 0, snap: busy},
				{at: time.Minute, snap: busy},
				{at: 70 * time.Second, snap: calm},
				{at: 89 * time.Second, snap: calm},
				{at: 95 * time.Second, snap: busy},
			},
			wantActive:   []Alert{cpuAlert},
			wantNotified: []string{"cpu "},
		},
		{
			name: "alert quiet for 30 s closes into recent, until is the last time the rule held",
			steps: []step{
				{at: 0, snap: busy}, {at: time.Minute, snap: busy}, {at: 62 * time.Second, snap: calm}, {at: 90 * time.Second, snap: calm},
			},
			wantRecent:   []Alert{cpuClosed},
			wantNotified: []string{"cpu "},
		},
		{
			name: "alert reopening within the hour is shown but not notified again",
			steps: []step{
				{at: 0, snap: busy},
				{at: time.Minute, snap: busy},
				{at: 100 * time.Second, snap: calm},
				{at: 10 * time.Minute, snap: busy},
				{at: 11 * time.Minute, snap: busy},
			},
			wantActive:   []Alert{cpuAgain},
			wantRecent:   []Alert{cpuClosed},
			wantNotified: []string{"cpu "},
		},
		{
			name: "alert reopening after the hour notifies again",
			steps: []step{
				{at: 0, snap: busy},
				{at: time.Minute, snap: busy},
				{at: 100 * time.Second, snap: calm},
				{at: 60 * time.Minute, snap: busy},
				{at: 61 * time.Minute, snap: busy},
			},
			wantActive:   []Alert{cpuNextHour},
			wantRecent:   []Alert{cpuClosed},
			wantNotified: []string{"cpu ", "cpu "},
		},
		{
			name: "alerts switched off in settings close the open alert and raise no new one",
			steps: []step{
				{at: 0, snap: busy},
				{at: time.Minute, snap: busy},
				{at: 62 * time.Second, snap: busy, off: true},
				{at: 3 * time.Minute, snap: busy, off: true},
				{at: 5 * time.Minute, snap: busy, off: true},
			},
			wantRecent:   []Alert{cpuClosed},
			wantNotified: []string{"cpu "},
		},
		{
			name: "busy app held for under five minutes is not an alert yet",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}}}},
				{at: 2 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}}}},
				{at: 299 * time.Second, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}}}},
			},
		},
		{
			name: "busy app needs five minutes, and each app is its own alert",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}}}},
				{at: 2 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}, {Name: "xz", CPU: 300}}}},
				{at: 299 * time.Second, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}, {Name: "xz", CPU: 300}}}},
				{at: 5 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}, {Name: "xz", CPU: 300}}}},
				{at: 7 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "yes", CPU: 200}, {Name: "xz", CPU: 300}}}},
			},
			wantActive: []Alert{
				{
					ID: fmt.Sprintf("app_cpu:yes:%d", t0.Unix()), Kind: KindAppCPU, App: "yes", Since: t0,
					Params: map[string]any{"app": "yes", "value": 200.0, "minutes": 5.0},
				},
				{
					ID: fmt.Sprintf("app_cpu:xz:%d", t0.Add(2*time.Minute).Unix()), Kind: KindAppCPU, App: "xz", Since: t0.Add(2 * time.Minute),
					Params: map[string]any{"app": "xz", "value": 300.0, "minutes": 5.0},
				},
			},
			wantNotified: []string{
				"app_cpu yes", "app_cpu xz",
			},
		},
		{
			name: "app memory doubling after warm-up is an alert",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: gib / 2}}}},
				{at: 4 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 3 * gib / 2}}}},
				{at: 10 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 4 * gib}}}},
				{at: 11 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 4 * gib}}}},
			},
			wantActive: []Alert{{
				ID: fmt.Sprintf("app_memory:Chrome:%d", t0.Add(10*time.Minute).Unix()), Kind: KindAppMemory, App: "Chrome",
				Since: t0.Add(10 * time.Minute), Params: map[string]any{"app": "Chrome", "value": 4.0, "low": 1.5, "minutes": 30.0},
			}},
			wantNotified: []string{"app_memory Chrome"},
		},
		{
			name: "app memory growing while the app launches is not an alert",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: gib / 10}}}},
				{at: 2 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: 3 * gib}}}},
				{at: 4 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: 3 * gib}}}},
				{at: 6 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: 3 * gib}}}},
				{at: 8 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: 3 * gib}}}},
			},
		},
		{
			name: "app that quit and came back warms up again",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: gib}}}},
				{at: 10 * time.Minute, snap: calm},
				{at: 11 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: gib / 10}}}},
				{at: 13 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: 3 * gib}}}},
				{at: 15 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Xcode", RSS: 3 * gib}}}},
			},
		},
		{
			name: "app memory doubling over more than 30 minutes is not an alert",
			steps: []step{
				{at: 0, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: gib}}}},
				{at: 6 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: gib}}}},
				{at: 37 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 3 * gib}}}},
				{at: 39 * time.Minute, snap: collector.Snapshot{Apps: []collector.App{{Name: "Chrome", RSS: 3 * gib}}}},
			},
		},
		{
			name: "muting an app closes its open alert at once, a device of the same name keeps its own",
			steps: []step{
				{at: 0, snap: hog}, {at: 5 * time.Minute, snap: hog}, {at: 5*time.Minute + 2*time.Second, snap: hog, muted: []string{"yes"}},
			},
			wantActive: []Alert{{
				ID: fmt.Sprintf("bt_battery:yes:%d", t0.Unix()), Kind: KindBTBattery, App: "yes", Since: t0,
				Params: map[string]any{"app": "yes", "value": 3},
			}},
			wantRecent: []Alert{{
				ID: fmt.Sprintf("app_cpu:yes:%d", t0.Unix()), Kind: KindAppCPU, App: "yes", Since: t0, Until: t0.Add(5 * time.Minute),
				Params: map[string]any{"app": "yes", "value": 200.0, "minutes": 5.0},
			}},
			wantNotified: []string{"bt_battery yes", "app_cpu yes"},
		},
		{
			name:         "clock stepping back drops the open alert instead of keeping it for the length of the step",
			steps:        []step{{at: 0, snap: busy}, {at: time.Minute, snap: busy}, {at: -time.Hour, snap: calm}},
			wantNotified: []string{"cpu "},
		},
		{
			name:  "rule held across a backward clock step fires after its hold, not after the step",
			steps: []step{{at: 0, snap: busy}, {at: -time.Hour, snap: busy}, {at: -time.Hour + time.Minute, snap: busy}},
			wantActive: []Alert{{
				ID: fmt.Sprintf("cpu::%d", t0.Add(-time.Hour).Unix()), Kind: KindCPU, Since: t0.Add(-time.Hour),
				Params: map[string]any{"value": 95.0, "limit": 90},
			}},
			wantNotified: []string{"cpu "},
		},
		{
			name:         "recent keeps the newest 20, newest first",
			steps:        cycles,
			wantRecent:   lastTwenty,
			wantNotified: []string{"cpu ", "cpu "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var notified []string
			e := New(func(a Alert) { notified = append(notified, a.Kind+" "+a.App) }, nil)

			for _, st := range tt.steps {
				cfg := settings.Default()
				if tt.cfg != nil {
					tt.cfg(&cfg)
				}
				cfg.Alerts, cfg.AlertMuted = !st.off, st.muted
				st.snap.Time = t0.Add(st.at)
				e.Check(&st.snap, cfg)
			}

			assert.Equal(t, tt.wantActive, e.Active())
			assert.Equal(t, tt.wantRecent, e.Recent())
			assert.Equal(t, tt.wantNotified, notified)
		})
	}
}
