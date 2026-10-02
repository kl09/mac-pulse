package alerts

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
)

var nan = float32(math.NaN())

// same compares two series bucket by bucket: NaN, a bucket without a sample, equals itself here.
func same(t *testing.T, want, got []float32) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		if math.IsNaN(float64(want[i])) {
			assert.True(t, math.IsNaN(float64(got[i])), "bucket %d is %v, want no sample", i, got[i])
			continue
		}
		assert.InDelta(t, want[i], got[i], 1e-4, "bucket %d", i)
	}
}

func TestSeries_add(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(1_000_000, 0)
	type sample struct {
		at time.Duration
		v  float64
	}
	// A sample every 10 s for 40 minutes and one more: the 241st does not fit.
	long := make([]sample, 0, maxPoints+1)
	for i := range maxPoints + 1 {
		long = append(long, sample{at: time.Duration(i) * 10 * time.Second, v: float64(i)})
	}
	halved := make([]float32, maxPoints/2+1)
	for i := range halved {
		halved[i] = float32(2*i) + 0.5
	}
	halved[maxPoints/2] = maxPoints

	tests := []struct {
		name      string
		samples   []sample
		wantStart int64
		wantStep  int64
		want      []float32
	}{
		{
			name:      "samples of one bucket average, the start sits on a multiple of the step",
			samples:   []sample{{3 * time.Second, 10}, {5 * time.Second, 20}, {9 * time.Second, 60}, {12 * time.Second, 7}},
			wantStart: 1_000_000, wantStep: 10, want: []float32{30, 7},
		},
		{
			name:      "bucket nothing was sampled in stays empty",
			samples:   []sample{{0, 1}, {35 * time.Second, 2}},
			wantStart: 1_000_000, wantStep: 10, want: []float32{1, nan, nan, 2},
		},
		{
			name:      "sample from before the newest bucket is dropped",
			samples:   []sample{{0, 1}, {20 * time.Second, 2}, {5 * time.Second, 99}, {-time.Hour, 99}},
			wantStart: 1_000_000, wantStep: 10, want: []float32{1, nan, 2},
		},
		{
			name:      "the 241st bucket merges the buckets in pairs and doubles the step",
			samples:   long,
			wantStart: 1_000_000, wantStep: 20, want: halved,
		},
		{
			name:      "a gap of a day halves until it fits: never more than 240 buckets",
			samples:   []sample{{0, 4}, {10 * time.Second, 8}, {24 * time.Hour, 1}},
			wantStart: 1_000_000, wantStep: 640, want: append(append([]float32{6}, slices.Repeat([]float32{nan}, 134)...), 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var s Series
			for _, sm := range tt.samples {
				s.add(t0.Add(sm.at), sm.v)
			}

			assert.Equal(t, tt.wantStart, s.Start)
			assert.Equal(t, tt.wantStep, s.Step)
			assert.LessOrEqual(t, len(s.V), maxPoints)
			same(t, tt.want, s.V)
		})
	}
}

func TestSeries_slide(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(1_000_000, 0)
	tests := []struct {
		name string
		// at are the sample times; the value of a sample is its index.
		at        []time.Duration
		wantStart int64
		want      []float32
	}{
		{name: "short pre-roll keeps everything", at: []time.Duration{0, 10 * time.Second}, wantStart: 1_000_000, want: []float32{0, 1}},
		{
			name:      "past ten minutes the oldest bucket leaves and the step stays",
			at:        []time.Duration{0, 590 * time.Second, 600 * time.Second, 610 * time.Second},
			wantStart: 1_000_020, want: append(slices.Repeat([]float32{nan}, 57), 1, 2, 3),
		},
		{name: "gap longer than the pre-roll starts over", at: []time.Duration{0, time.Hour}, wantStart: 1_003_600, want: []float32{1}},
		{name: "backward clock step starts over", at: []time.Duration{time.Hour, 0}, wantStart: 1_000_000, want: []float32{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var s Series
			for i, at := range tt.at {
				s.slide(t0.Add(at), float64(i))
			}

			assert.Equal(t, tt.wantStart, s.Start)
			assert.EqualValues(t, seriesStep, s.Step)
			same(t, tt.want, s.V)
		})
	}
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestCapture(t *testing.T) {
	t.Parallel()

	const gib = 1 << 30
	snap := collector.Snapshot{
		CPU: collector.CPU{Total: 93, User: 70, System: 23}, CPUTemp: 97.5, Thermal: 2,
		Memory: collector.Memory{
			Total: 16 * gib, Used: 12 * gib, App: 8 * gib, Wired: 2 * gib,
			Compressed: gib, Cached: 3 * gib, Free: gib, SwapUsed: 5 * gib,
		},
		Disk: collector.Disk{Total: 1000, Free: 42, WriteRate: 9e6},
		Battery: collector.Battery{
			Present: true, Percent: 12, Power: -18.5, TimeRemaining: 40 * time.Minute,
		},
		Bluetooth: []collector.BluetoothDevice{
			{Name: "Mouse", Levels: []collector.BatteryLevel{{Part: "main", Percent: 80}}},
			{Name: "AirPods", Levels: []collector.BatteryLevel{{Part: "left", Percent: 60}, {Part: "right", Percent: 9}}},
		},
		Apps: []collector.App{
			{Name: "idle"},
			{
				Name: "Chrome", BundlePath: "/Applications/Chrome.app", PIDs: []int32{1, 2, 3},
				CPU: 212, RSS: 4 * gib, DiskWriteRate: 100, EnergyMW: 900,
				Processes: []collector.Process{
					{PID: 1, Name: "Chrome", CPU: 12, RSS: 3 * gib},
					{PID: 2, Name: "Helper", CPU: 200, RSS: gib},
					{PID: 3, Name: "GPU"},
				},
			},
			{Name: "a", CPU: 1, RSS: 1},
			{Name: "b", CPU: 2, RSS: 2},
			{Name: "c", CPU: 3, RSS: 3},
			{Name: "d", CPU: 4, RSS: 4},
			{Name: "Docker", CPU: 30, RSS: 6 * gib, DiskWriteRate: 5e6, EnergyMW: 300},
		},
	}
	byCPU := []Row{
		{Name: "Chrome", BundlePath: "/Applications/Chrome.app", Value: 212},
		{Name: "Docker", Value: 30},
		{Name: "d", Value: 4},
		{Name: "c", Value: 3},
		{Name: "b", Value: 2},
	}
	byMemory := []Row{
		{Name: "Docker", Value: 6 * gib},
		{Name: "Chrome", BundlePath: "/Applications/Chrome.app", Value: 4 * gib},
		{Name: "d", Value: 4},
		{Name: "c", Value: 3},
		{Name: "b", Value: 2},
	}
	memoryFacts := []Fact{
		{Key: "mem_app", Value: 8 * gib, Unit: UnitBytes},
		{Key: "mem_wired", Value: 2 * gib, Unit: UnitBytes},
		{Key: "mem_compressed", Value: gib, Unit: UnitBytes},
		{Key: "mem_cached", Value: 3 * gib, Unit: UnitBytes},
		{Key: "mem_free", Value: gib, Unit: UnitBytes},
		{Key: "mem_swap", Value: 5 * gib, Unit: UnitBytes},
	}
	heat := []Fact{{Key: "thermal", Value: 2, Unit: UnitState}, {Key: "cpu_temp", Value: 97.5, Unit: UnitCelsius}}

	tests := []struct {
		name      string
		s         subject
		wantUnit  string
		wantValue float64
		// noValue: the sample has no reading of the metric.
		noValue bool
		want    Context
	}{
		{
			name: "cpu: the split and the apps by CPU", s: subject{kind: KindCPU}, wantUnit: UnitPercent, wantValue: 93,
			want: Context{TopUnit: UnitPercent, Top: byCPU, Facts: []Fact{
				{Key: "cpu_user", Value: 70, Unit: UnitPercent}, {Key: "cpu_system", Value: 23, Unit: UnitPercent},
			}},
		},
		{
			name: "temp: the thermal state next to it", s: subject{kind: KindTemp}, wantUnit: UnitCelsius, wantValue: 97.5,
			want: Context{TopUnit: UnitPercent, Top: byCPU, Facts: heat},
		},
		{
			name: "thermal goes by the CPU temperature", s: subject{kind: KindThermal}, wantUnit: UnitCelsius, wantValue: 97.5,
			want: Context{TopUnit: UnitPercent, Top: byCPU, Facts: heat},
		},
		{
			name: "memory pressure: the breakdown and the apps by memory", s: subject{kind: KindMemory}, wantUnit: UnitPercent, wantValue: 75,
			want: Context{TopUnit: UnitBytes, Top: byMemory, Facts: memoryFacts},
		},
		{
			name: "memory used", s: subject{kind: KindMemoryUsed}, wantUnit: UnitPercent, wantValue: 75,
			want: Context{TopUnit: UnitBytes, Top: byMemory, Facts: memoryFacts},
		},
		{
			name: "swap is bytes", s: subject{kind: KindSwap}, wantUnit: UnitBytes, wantValue: 5 * gib,
			want: Context{TopUnit: UnitBytes, Top: byMemory, Facts: memoryFacts},
		},
		{
			name: "disk: the free space and who writes", s: subject{kind: KindDisk}, wantUnit: UnitPercent, wantValue: 4.2,
			want: Context{
				TopUnit: UnitBytesPerS,
				Top:     []Row{{Name: "Docker", Value: 5e6}, {Name: "Chrome", BundlePath: "/Applications/Chrome.app", Value: 100}},
				Facts: []Fact{
					{Key: "disk_free", Value: 42, Unit: UnitBytes},
					{Key: "disk_total", Value: 1000, Unit: UnitBytes},
					{Key: "disk_write", Value: 9e6, Unit: UnitBytesPerS},
				},
			},
		},
		{
			name: "battery: level, draw, time left and who draws power", s: subject{kind: KindBatteryLow}, wantUnit: UnitPercent, wantValue: 12,
			want: Context{
				TopUnit: UnitMilliwatts,
				Top:     []Row{{Name: "Chrome", BundlePath: "/Applications/Chrome.app", Value: 900}, {Name: "Docker", Value: 300}},
				Facts: []Fact{
					{Key: "bat_level", Value: 12, Unit: UnitPercent},
					{Key: "bat_draw", Value: 18.5, Unit: UnitWatts},
					{Key: "bat_left", Value: 2400, Unit: UnitSeconds},
				},
			},
		},
		{
			name: "bluetooth: the parts of the device, no apps",
			s:    subject{kind: KindBTBattery, app: "AirPods"}, wantUnit: UnitPercent, wantValue: 9,
			want: Context{TopUnit: UnitPercent, Facts: []Fact{
				{Key: "bt_left", Value: 60, Unit: UnitPercent}, {Key: "bt_right", Value: 9, Unit: UnitPercent},
			}},
		},
		{
			name: "app cpu: its processes by CPU", s: subject{kind: KindAppCPU, app: "Chrome"}, wantUnit: UnitPercent, wantValue: 212,
			want: Context{
				BundlePath: "/Applications/Chrome.app", PIDs: 3, TopUnit: UnitPercent, Top: byCPU,
				Procs: []Row{{Name: "Helper", PID: 2, Value: 200}, {Name: "Chrome", PID: 1, Value: 12}, {Name: "GPU", PID: 3}},
			},
		},
		{
			name: "app memory: its processes by memory", s: subject{kind: KindAppMemory, app: "Chrome"}, wantUnit: UnitBytes, wantValue: 4 * gib,
			want: Context{
				BundlePath: "/Applications/Chrome.app", PIDs: 3, TopUnit: UnitBytes, Top: byMemory,
				Procs: []Row{{Name: "Chrome", PID: 1, Value: 3 * gib}, {Name: "Helper", PID: 2, Value: gib}, {Name: "GPU", PID: 3}},
			},
		},
		{
			name: "own rule on memory", s: subject{kind: KindAppRule, app: "Chrome", metric: "memory"}, wantUnit: UnitBytes, wantValue: 4 * gib,
			want: Context{
				BundlePath: "/Applications/Chrome.app", PIDs: 3, TopUnit: UnitBytes, Top: byMemory,
				Procs: []Row{{Name: "Chrome", PID: 1, Value: 3 * gib}, {Name: "Helper", PID: 2, Value: gib}, {Name: "GPU", PID: 3}},
			},
		},
		{
			name: "own rule on cpu", s: subject{kind: KindAppRule, app: "Chrome", metric: "cpu"}, wantUnit: UnitPercent, wantValue: 212,
			want: Context{
				BundlePath: "/Applications/Chrome.app", PIDs: 3, TopUnit: UnitPercent, Top: byCPU,
				Procs: []Row{{Name: "Helper", PID: 2, Value: 200}, {Name: "Chrome", PID: 1, Value: 12}, {Name: "GPU", PID: 3}},
			},
		},
		{
			name: "app that is gone has no value and no processes", s: subject{kind: KindAppCPU, app: "quit"}, wantUnit: UnitPercent, noValue: true,
			want: Context{TopUnit: UnitPercent, Top: byCPU},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, ok := value(&snap, tt.s)

			assert.Equal(t, tt.want, capture(&snap, tt.s))
			assert.Equal(t, tt.wantUnit, unitOf(tt.s))
			assert.Equal(t, !tt.noValue, ok)
			assert.InDelta(t, tt.wantValue, v, 1e-9)
		})
	}
}

// The life of one record: the pre-roll before it, the hold, the fire, the peak and the average
// while the rule holds, the tail after it lets go, the close.
//
//nolint:funlen // one table for the function under test; the length is the cases.
func TestEngine_Record(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(1_000_000, 0)
	cpu := func(total float64) collector.Snapshot {
		return collector.Snapshot{CPU: collector.CPU{Total: total, User: total}}
	}
	yes := func(load float64) collector.Snapshot {
		return collector.Snapshot{Apps: []collector.App{{
			Name: "yes", CPU: load, PIDs: []int32{7},
			Processes: []collector.Process{{PID: 7, Name: "yes", CPU: load}},
		}}}
	}
	type step struct {
		at   time.Duration
		snap collector.Snapshot
	}
	cpuID, yesID := "cpu::1000020", "app_cpu:yes:1000000"

	tests := []struct {
		name   string
		steps  []step
		id     string
		absent bool
		want   Record
	}{
		{name: "unknown id", steps: []step{{0, cpu(95)}}, id: "cpu::1", absent: true},
		{
			name:  "rule that holds but has not fired yet is no alert",
			steps: []step{{0, cpu(50)}, {20 * time.Second, cpu(95)}}, id: cpuID, absent: true,
		},
		{
			name: "fired: the value at that moment and the context, the samples start before the alert",
			steps: []step{
				{0, cpu(50)},
				{10 * time.Second, cpu(60)},
				{20 * time.Second, cpu(95)},
				{50 * time.Second, cpu(91)},
				{80 * time.Second, cpu(99)},
			},
			id: cpuID,
			want: Record{
				Alert: Alert{ID: cpuID, Kind: KindCPU, Since: t0.Add(20 * time.Second), Params: map[string]any{"value": 99.0, "limit": 90}},
				Detail: &Detail{
					Unit: UnitPercent, Limit: 90, Hold: time.Minute, FiredAt: t0.Add(80 * time.Second),
					Fired: 99, Peak: 99, Last: 99, Sum: 285, N: 3,
					Series: Series{Start: 1_000_000, Step: 10, V: []float32{50, 60, 95, nan, nan, 91, nan, nan, 99}, last: 1},
					Context: Context{TopUnit: UnitPercent, Facts: []Fact{
						{Key: "cpu_user", Value: 99, Unit: UnitPercent}, {Key: "cpu_system", Unit: UnitPercent},
					}},
				},
			},
		},
		{
			name: "after the fire the peak, the average and the newest value move on; the fired value and the context stay",
			steps: []step{
				{20 * time.Second, cpu(95)}, {80 * time.Second, cpu(92)}, {90 * time.Second, cpu(100)}, {100 * time.Second, cpu(93)},
			},
			id: cpuID,
			want: Record{
				Alert: Alert{ID: cpuID, Kind: KindCPU, Since: t0.Add(20 * time.Second), Params: map[string]any{"value": 93.0, "limit": 90}},
				Detail: &Detail{
					Unit: UnitPercent, Limit: 90, Hold: time.Minute, FiredAt: t0.Add(80 * time.Second),
					Fired: 92, Peak: 100, Last: 93, Sum: 380, N: 4,
					Series: Series{Start: 1_000_020, Step: 10, V: []float32{95, nan, nan, nan, nan, nan, 92, 100, 93}, last: 1},
					Context: Context{TopUnit: UnitPercent, Facts: []Fact{
						{Key: "cpu_user", Value: 92, Unit: UnitPercent}, {Key: "cpu_system", Unit: UnitPercent},
					}},
				},
			},
		},
		{
			name: "ended: until is the last time the rule held, the samples show the drop, the dip is not in the average",
			steps: []step{
				{20 * time.Second, cpu(95)},
				{80 * time.Second, cpu(97)},
				{90 * time.Second, cpu(40)},
				{100 * time.Second, cpu(30)},
				{110 * time.Second, cpu(20)},
			},
			id: cpuID,
			want: Record{
				Alert: Alert{
					ID: cpuID, Kind: KindCPU, Since: t0.Add(20 * time.Second), Until: t0.Add(80 * time.Second),
					Params: map[string]any{"value": 97.0, "limit": 90},
				},
				Detail: &Detail{
					Unit: UnitPercent, Limit: 90, Hold: time.Minute, FiredAt: t0.Add(80 * time.Second),
					Fired: 97, Peak: 97, Last: 30, Sum: 192, N: 2,
					Series: Series{Start: 1_000_020, Step: 10, V: []float32{95, nan, nan, nan, nan, nan, 97, 40, 30}, last: 1},
					Context: Context{TopUnit: UnitPercent, Facts: []Fact{
						{Key: "cpu_user", Value: 97, Unit: UnitPercent}, {Key: "cpu_system", Unit: UnitPercent},
					}},
				},
			},
		},
		{
			name:  "the sample that starts the rule counts once: its bucket is the mean of its samples",
			steps: []step{{0, cpu(10)}, {2 * time.Second, cpu(100)}, {62 * time.Second, cpu(100)}},
			id:    "cpu::1000002",
			want: Record{
				Alert: Alert{ID: "cpu::1000002", Kind: KindCPU, Since: t0.Add(2 * time.Second), Params: map[string]any{"value": 100.0, "limit": 90}},
				Detail: &Detail{
					Unit: UnitPercent, Limit: 90, Hold: time.Minute, FiredAt: t0.Add(62 * time.Second),
					Fired: 100, Peak: 100, Last: 100, Sum: 200, N: 2,
					Series: Series{Start: 1_000_000, Step: 10, V: []float32{55, nan, nan, nan, nan, nan, 100}, last: 1},
					Context: Context{TopUnit: UnitPercent, Facts: []Fact{
						{Key: "cpu_user", Value: 100, Unit: UnitPercent}, {Key: "cpu_system", Unit: UnitPercent},
					}},
				},
			},
		},
		{
			name:  "app alert: no samples from before it, its processes in the context",
			steps: []step{{-10 * time.Second, yes(20)}, {0, yes(200)}, {5 * time.Minute, yes(180)}},
			id:    yesID,
			want: Record{
				Alert: Alert{ID: yesID, Kind: KindAppCPU, App: "yes", Since: t0, Params: map[string]any{"app": "yes", "value": 180.0, "minutes": 5.0}},
				Detail: &Detail{
					Unit: UnitPercent, Limit: appCPUPercent, Hold: appCPUHold, FiredAt: t0.Add(5 * time.Minute),
					Fired: 180, Peak: 200, Last: 180, Sum: 380, N: 2,
					Series: Series{Start: 1_000_000, Step: 10, V: append(append([]float32{200}, slices.Repeat([]float32{nan}, 29)...), 180), last: 1},
					Context: Context{
						PIDs: 1, TopUnit: UnitPercent,
						Top: []Row{{Name: "yes", Value: 180}}, Procs: []Row{{Name: "yes", PID: 7, Value: 180}},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := New(func(Alert) {}, nil)
			for _, st := range tt.steps {
				st.snap.Time = t0.Add(st.at)
				e.Check(&st.snap, settings.Default())
			}

			got, ok := e.Record(tt.id)

			require.Equal(t, !tt.absent, ok)
			if tt.absent {
				return
			}
			same(t, tt.want.Detail.Series.V, got.Detail.Series.V)
			tt.want.Detail.Series.V, got.Detail.Series.V = nil, nil
			assert.Equal(t, tt.want, got)
		})
	}
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestEngine_Closed(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(1_000_000, 0)
	busy, calm := collector.Snapshot{CPU: collector.CPU{Total: 95}}, collector.Snapshot{}
	old := Record{Alert: Alert{ID: "disk::5", Kind: KindDisk, Since: time.Unix(5, 0), Until: time.Unix(9, 0)}}
	many := make([]Record, maxRecent+5)
	for i := range many {
		many[i] = old
	}
	// kept is a record as the engine writes one; each of the others differs from it in one
	// thing the engine never writes.
	stored := func(change func(*Record)) Record {
		r := Record{
			Alert: Alert{
				ID: "cpu::7", Kind: KindCPU, Since: time.Unix(7, 0), Until: time.Unix(8, 0), Params: map[string]any{"value": 95.0, "limit": 90},
			},
			Detail: &Detail{
				Unit: UnitPercent, Limit: 90, Fired: 95, Peak: 95, Last: 95, Sum: 95, N: 1,
				Series:  Series{Start: 0, Step: 10, V: []float32{95, nan}},
				Context: Context{Top: []Row{{Name: "yes", Value: 95}}, Facts: []Fact{{Key: "cpu_user", Value: 95, Unit: UnitPercent}}},
			},
		}
		change(&r)
		return r
	}
	kept := stored(func(*Record) {})
	tampered := []Record{
		kept,
		stored(func(r *Record) { r.Params["value"] = math.NaN() }),
		stored(func(r *Record) { r.Params["value"] = float32(1) }),
		stored(func(r *Record) { r.Kind = "evil" }),
		stored(func(r *Record) { r.Detail.Peak = math.Inf(1) }),
		stored(func(r *Record) { r.Detail.Series.V = make([]float32, maxPoints+1) }),
		stored(func(r *Record) { r.Detail.Series.Step = 0 }),
		stored(func(r *Record) { r.Detail.Context.Top = make([]Row, contextRows+1) }),
		stored(func(r *Record) { r.Detail.Context.Procs = []Row{{Value: math.NaN()}} }),
		stored(func(r *Record) { r.Detail.Context.Facts = make([]Fact, maxFacts+1) }),
		stored(func(r *Record) { r.Detail.Context.Facts[0].Value = math.Inf(-1) }),
		old,
	}
	type step struct {
		at   time.Duration
		snap collector.Snapshot
	}

	tests := []struct {
		name     string
		restored []Record
		steps    []step
		// wantIDs and wantUntil describe the records, newest first.
		wantIDs   []string
		wantUntil []time.Time
	}{
		{name: "nothing happened"},
		{
			name: "restored records come back, one without a detail too", restored: []Record{old},
			wantIDs: []string{"disk::5"}, wantUntil: []time.Time{old.Until},
		},
		{
			name: "more than 20 restored records are cut", restored: many,
			wantIDs: slices.Repeat([]string{"disk::5"}, maxRecent), wantUntil: slices.Repeat([]time.Time{old.Until}, maxRecent),
		},
		{
			name: "stored records the engine could not have written are dropped, the ones around them stay", restored: tampered,
			wantIDs: []string{"cpu::7", "disk::5"}, wantUntil: []time.Time{kept.Until, old.Until},
		},
		{
			name:     "open alert is saved as closed at the last time its rule held, above the older ones",
			restored: []Record{old},
			steps:    []step{{0, busy}, {time.Minute, busy}, {70 * time.Second, calm}},
			wantIDs:  []string{"cpu::1000000", "disk::5"}, wantUntil: []time.Time{t0.Add(time.Minute), old.Until},
		},
		{
			name:     "closed alert of this run sits above the restored ones",
			restored: []Record{old},
			steps:    []step{{0, busy}, {time.Minute, busy}, {2 * time.Minute, calm}},
			wantIDs:  []string{"cpu::1000000", "disk::5"}, wantUntil: []time.Time{t0.Add(time.Minute), old.Until},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := New(func(Alert) {}, tt.restored)
			for _, st := range tt.steps {
				st.snap.Time = t0.Add(st.at)
				e.Check(&st.snap, settings.Default())
			}

			var gotIDs []string
			var gotUntil []time.Time
			for _, r := range e.Closed() {
				gotIDs, gotUntil = append(gotIDs, r.ID), append(gotUntil, r.Until)
			}

			assert.Equal(t, tt.wantIDs, gotIDs)
			assert.Equal(t, tt.wantUntil, gotUntil)
			if len(tt.steps) == 0 {
				assert.Len(t, e.Recent(), len(tt.wantIDs), "what was not kept is not listed either")
			}
			if len(tt.restored) > 0 {
				got, ok := e.Record("disk::5")
				assert.True(t, ok)
				assert.Nil(t, got.Detail)
			}
		})
	}
}
