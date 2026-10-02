package store

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
)

//nolint:funlen // one table for the function under test; the length is the cases.
func TestStore_History(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 30, 12, 30, 30, 0, time.UTC)
	dozen := make([]collector.App, 12)
	wantDozen := make([]AppValue, topApps)
	for i := range dozen {
		dozen[i] = collector.App{Name: fmt.Sprintf("app%02d", i), CPU: float64(100 - i)}
	}
	for i := range wantDozen {
		wantDozen[i] = AppValue{Name: fmt.Sprintf("app%02d", i), Value: float64(100 - i)}
	}

	watts := func(v float64) *float64 { return &v }

	tests := []struct {
		name   string
		snaps  []collector.Snapshot
		usage  []netinspect.AppUsage
		metric string
		rng    string
		// wantPoints maps series name to bucket index to value; every other bucket is nil.
		wantPoints  map[string]map[int]float64
		wantUnits   map[string]string
		wantLen     int
		wantStart   time.Time
		wantStep    time.Duration
		wantTopApps []AppValue
		wantErr     error
	}{
		{name: "unknown metric", metric: "fans", rng: "1h", wantErr: ErrUnknownMetric},
		{name: "unknown range", metric: "cpu", rng: "2y", wantErr: ErrUnknownRange},
		{
			name:       "empty store has only gaps",
			metric:     "cpu",
			rng:        "24h",
			wantPoints: map[string]map[int]float64{"total": {}},
			wantUnits:  map[string]string{"total": "percent"},
			wantLen:    144,
			wantStart:  time.Date(2026, 9, 29, 12, 40, 0, 0, time.UTC),
			wantStep:   10 * time.Minute,
		},
		{
			name: "1h averages each minute and leaves a gap where nothing ran",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 9, 30, 11, 30, 59, 0, time.UTC), CPU: collector.CPU{Total: 99}},
				{Time: time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC), CPU: collector.CPU{Total: 5}},
				{Time: time.Date(2026, 9, 30, 12, 28, 10, 0, time.UTC), CPU: collector.CPU{Total: 10}},
				{Time: time.Date(2026, 9, 30, 12, 28, 40, 0, time.UTC), CPU: collector.CPU{Total: 30}},
				{Time: time.Date(2026, 9, 30, 12, 30, 10, 0, time.UTC), CPU: collector.CPU{Total: 50}},
			},
			metric:     "cpu",
			rng:        "1h",
			wantPoints: map[string]map[int]float64{"total": {0: 5, 57: 20, 59: 50}},
			wantUnits:  map[string]string{"total": "percent"},
			wantLen:    60,
			wantStart:  time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:   time.Minute,
		},
		{
			name: "12h averages the minutes of a five-minute bucket",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 9, 30, 12, 24, 59, 0, time.UTC), Memory: collector.Memory{Used: 8 << 30}},
				{Time: time.Date(2026, 9, 30, 12, 25, 0, 0, time.UTC), Memory: collector.Memory{Used: 1 << 30}},
				{Time: time.Date(2026, 9, 30, 12, 26, 0, 0, time.UTC), Memory: collector.Memory{Used: 2 << 30}},
				{Time: time.Date(2026, 9, 30, 12, 29, 59, 0, time.UTC), Memory: collector.Memory{Used: 6 << 30}},
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), Memory: collector.Memory{Used: 4 << 30}},
			},
			metric:     "memory",
			rng:        "12h",
			wantPoints: map[string]map[int]float64{"used": {141: 8 << 30, 142: 3 << 30, 143: 4 << 30}},
			wantUnits:  map[string]string{"used": "bytes"},
			wantLen:    144,
			wantStart:  time.Date(2026, 9, 30, 0, 35, 0, 0, time.UTC),
			wantStep:   5 * time.Minute,
		},
		{
			name: "network has a series per direction and ranks apps by total bytes",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), Network: collector.Network{DownRate: 3000, UpRate: 100}},
				{Time: time.Date(2026, 9, 30, 12, 30, 2, 0, time.UTC), Network: collector.Network{DownRate: 1000, UpRate: 300}},
			},
			usage: []netinspect.AppUsage{
				{App: "curl", Down: 100, Up: 10},
				{App: "Safari", BundlePath: "/Applications/Safari.app", Down: 5000, Up: 500},
				{App: "idle"},
			},
			metric:     "network",
			rng:        "1h",
			wantPoints: map[string]map[int]float64{"down": {59: 2000}, "up": {59: 200}},
			wantUnits:  map[string]string{"down": "bytes_per_s", "up": "bytes_per_s"},
			wantLen:    60,
			wantStart:  time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:   time.Minute,
			wantTopApps: []AppValue{
				{Name: "Safari", BundlePath: "/Applications/Safari.app", Value: 5500},
				{Name: "curl", Value: 110},
			},
		},
		{
			name: "cpu ranks apps by their average over all samples, closed hours included",
			snaps: []collector.Snapshot{
				{
					Time: time.Date(2026, 9, 30, 11, 50, 0, 0, time.UTC),
					Apps: []collector.App{{Name: "yes", CPU: 100}, {Name: "Xcode", BundlePath: "/Applications/Xcode.app", CPU: 30}},
				},
				{
					Time: time.Date(2026, 9, 30, 12, 10, 0, 0, time.UTC),
					Apps: []collector.App{{Name: "Xcode", BundlePath: "/Applications/Xcode.app", CPU: 90}, {Name: "idle"}},
				},
			},
			metric:     "cpu",
			rng:        "1h",
			wantPoints: map[string]map[int]float64{"total": {19: 0, 39: 0}},
			wantUnits:  map[string]string{"total": "percent"},
			wantLen:    60,
			wantStart:  time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:   time.Minute,
			wantTopApps: []AppValue{
				{Name: "Xcode", BundlePath: "/Applications/Xcode.app", Value: 60},
				{Name: "yes", Value: 50},
			},
		},
		{
			name: "memory ranks apps by average bytes and skips hours outside the range",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC), Apps: []collector.App{{Name: "old", RSS: 9 << 30}}},
				{
					Time: time.Date(2026, 9, 30, 12, 10, 0, 0, time.UTC),
					Apps: []collector.App{{Name: "Chrome", RSS: 2 << 30}, {Name: "Slack", RSS: 1 << 30}},
				},
				{Time: time.Date(2026, 9, 30, 12, 10, 1, 0, time.UTC), Apps: []collector.App{{Name: "Chrome", RSS: 4 << 30}}},
			},
			metric:      "memory",
			rng:         "1h",
			wantPoints:  map[string]map[int]float64{"used": {39: 0}},
			wantUnits:   map[string]string{"used": "bytes"},
			wantLen:     60,
			wantStart:   time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:    time.Minute,
			wantTopApps: []AppValue{{Name: "Chrome", Value: 3 << 30}, {Name: "Slack", Value: 1 << 29}},
		},
		{
			name:        "at most ten apps are ranked",
			snaps:       []collector.Snapshot{{Time: time.Date(2026, 9, 30, 12, 10, 0, 0, time.UTC), Apps: dozen}},
			metric:      "cpu",
			rng:         "1h",
			wantPoints:  map[string]map[int]float64{"total": {39: 0}},
			wantUnits:   map[string]string{"total": "percent"},
			wantLen:     60,
			wantStart:   time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:    time.Minute,
			wantTopApps: wantDozen,
		},
		{
			name: "metric without per-app data has no ranking",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), CPUTemp: 61, Apps: []collector.App{{Name: "yes", CPU: 100}}},
			},
			metric:     "temp",
			rng:        "1h",
			wantPoints: map[string]map[int]float64{"cpu": {59: 61}},
			wantUnits:  map[string]string{"cpu": "celsius"},
			wantLen:    60,
			wantStart:  time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:   time.Minute,
		},
		{
			name: "power has three series and a minute without a reading is a gap, not 0 W",
			snaps: []collector.Snapshot{
				{
					Time:  time.Date(2026, 9, 30, 12, 28, 0, 0, time.UTC),
					Power: collector.Power{SystemW: watts(12), AdapterW: watts(60), CPUW: watts(2), GPUW: watts(0.5)},
				},
				{Time: time.Date(2026, 9, 30, 12, 28, 30, 0, time.UTC), Power: collector.Power{SystemW: watts(14), CPUW: watts(4), GPUW: watts(0.5)}},
				{Time: time.Date(2026, 9, 30, 12, 29, 0, 0, time.UTC)},
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), Power: collector.Power{CPUW: watts(1)}},
			},
			metric:     "power",
			rng:        "1h",
			wantPoints: map[string]map[int]float64{"system": {57: 13}, "cpu": {57: 3, 59: 1}, "gpu": {57: 0.5}},
			wantUnits:  map[string]string{"system": "watts", "cpu": "watts", "gpu": "watts"},
			wantLen:    60,
			wantStart:  time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:   time.Minute,
		},
		{
			name: "90d averages the hours of a twelve-hour bucket and ranks no apps: they are kept for 30 days",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 7, 2, 23, 59, 0, 0, time.UTC), CPU: collector.CPU{Total: 99}},
				{Time: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), CPU: collector.CPU{Total: 5}},
				{Time: time.Date(2026, 9, 30, 0, 10, 0, 0, time.UTC), CPU: collector.CPU{Total: 10}},
				{Time: time.Date(2026, 9, 30, 0, 50, 0, 0, time.UTC), CPU: collector.CPU{Total: 20}},
				{
					Time: time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC), CPU: collector.CPU{Total: 45},
					Apps: []collector.App{{Name: "yes", CPU: 100}},
				},
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), CPU: collector.CPU{Total: 50}},
			},
			metric:     "cpu",
			rng:        "90d",
			wantPoints: map[string]map[int]float64{"total": {0: 5, 178: 30, 179: 50}},
			wantUnits:  map[string]string{"total": "percent"},
			wantLen:    180,
			wantStart:  time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
			wantStep:   12 * time.Hour,
		},
		{
			name: "1y reaches past the 30 days of minutes, in two-day buckets",
			snaps: []collector.Snapshot{
				{Time: time.Date(2025, 9, 30, 23, 0, 0, 0, time.UTC), Memory: collector.Memory{Used: 9 << 30}},
				{Time: time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC), Memory: collector.Memory{Used: 1 << 30}},
				{Time: time.Date(2025, 10, 2, 23, 0, 0, 0, time.UTC), Memory: collector.Memory{Used: 3 << 30}},
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), Memory: collector.Memory{Used: 4 << 30}},
			},
			metric:     "memory",
			rng:        "1y",
			wantPoints: map[string]map[int]float64{"used": {0: 2 << 30, 182: 4 << 30}},
			wantUnits:  map[string]string{"used": "bytes"},
			wantLen:    183,
			wantStart:  time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC),
			wantStep:   48 * time.Hour,
		},
		{
			name: "non-finite sample leaves the minute's average alone",
			snaps: []collector.Snapshot{
				{Time: time.Date(2026, 9, 30, 12, 30, 0, 0, time.UTC), CPU: collector.CPU{Total: 10}},
				{Time: time.Date(2026, 9, 30, 12, 30, 2, 0, time.UTC), CPU: collector.CPU{Total: math.NaN()}},
				{Time: time.Date(2026, 9, 30, 12, 30, 4, 0, time.UTC), CPU: collector.CPU{Total: math.Inf(1)}},
			},
			metric:     "cpu",
			rng:        "1h",
			wantPoints: map[string]map[int]float64{"total": {59: 10}},
			wantUnits:  map[string]string{"total": "percent"},
			wantLen:    60,
			wantStart:  time.Date(2026, 9, 30, 11, 31, 0, 0, time.UTC),
			wantStep:   time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			require.NoError(t, err)
			s.now = func() time.Time { return now }
			for i := range tt.snaps {
				s.Add(&tt.snaps[i])
			}
			s.AddUsage(tt.usage)

			got, err := s.History(tt.metric, tt.rng)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.metric, got.Metric)
			assert.Equal(t, tt.rng, got.Range)
			assert.True(t, tt.wantStart.Equal(got.Start), "start %s", got.Start.UTC())
			assert.Equal(t, tt.wantStep, got.Step)
			assert.Equal(t, tt.wantTopApps, got.TopApps)
			require.Len(t, got.Series, len(tt.wantPoints))
			for _, series := range got.Series {
				want, ok := tt.wantPoints[series.Name]
				require.True(t, ok, "unexpected series %q", series.Name)
				assert.Equal(t, tt.wantUnits[series.Name], series.Unit)
				require.Len(t, series.Points, tt.wantLen)
				gotPoints := map[int]float64{}
				for i, p := range series.Points {
					if p != nil {
						gotPoints[i] = *p
					}
				}
				assert.Equal(t, want, gotPoints, "series %q", series.Name)
			}
		})
	}
}
