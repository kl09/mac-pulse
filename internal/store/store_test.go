package store

import (
	"bytes"
	"encoding/gob"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/alerts"
	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
)

//nolint:funlen // one table for the function under test; the length is the cases.
func TestOpen(t *testing.T) {
	t.Parallel()

	var future, valid, noHourly, withHourly bytes.Buffer
	require.NoError(t, gob.NewEncoder(&future).Encode(state{Version: version + 1, Day: "2026-09-30", DiskWritten: 7}))
	require.NoError(t, gob.NewEncoder(&valid).Encode(state{Version: version, Day: "2026-09-30", DiskWritten: 7}))
	// What v3 wrote: the same version, no Hourly field.
	minutes := []minute{
		{At: 29846160, N: 1, V: [seriesCount]float32{seriesCPU: 40}}, {At: 29846161, N: 3, V: [seriesCount]float32{seriesCPU: 80}},
	}
	require.NoError(t, gob.NewEncoder(&noHourly).Encode(struct {
		Version int
		Minutes []minute
	}{Version: version, Minutes: minutes}))
	require.NoError(t, gob.NewEncoder(&withHourly).Encode(state{
		Version: version, Minutes: minutes, Hourly: []minute{{At: 497000, N: 5, V: [seriesCount]float32{seriesCPU: 5}}},
	}))

	v1, err := os.ReadFile("testdata/history-v1.gob")
	require.NoError(t, err)

	v1Today := Today{DiskWritten: 2000, NetApps: []AppTotal{{Name: "Safari", BundlePath: "/Applications/Safari.app", Down: 500, Up: 50}}}
	v1Hourly := []minute{{At: 497436, N: 3, V: [seriesCount]float32{
		seriesCPU: 20, seriesMemory: 8 << 30, seriesGPU: 7, seriesTemp: 50, seriesBattery: 80,
	}}}

	tests := []struct {
		name string
		// file is written as history.gob unless nil, oldBackup as history.gob.bak.
		file      []byte
		oldBackup []byte
		readOnly  bool
		wantErr   error
		wantToday Today
		// wantCPU is the cpu average of every loaded minute, wantHourly of every hour.
		wantCPU    []float32
		wantHourly []minute
		wantBackup bool
		// wantKept: the file stays in place next to its backup.
		wantKept bool
		// wantFiles are the names in the directory after Open, when not nil.
		wantFiles []string
	}{
		{name: "missing file gives an empty store"},
		{name: "valid file loads", file: valid.Bytes(), wantToday: Today{DiskWritten: 7}},
		{name: "garbage is set aside", file: []byte("not a gob stream"), wantBackup: true},
		{name: "truncated file is set aside", file: valid.Bytes()[:valid.Len()/2], wantBackup: true},
		{name: "empty file is set aside", file: []byte{}, wantBackup: true},
		{name: "another version is set aside", file: future.Bytes(), wantBackup: true},
		{
			name: "version 1 file migrates with its minutes and leaves a copy", file: v1,
			wantToday: v1Today,
			wantCPU:   []float32{10, 20, 30}, wantBackup: true, wantKept: true,
			wantHourly: v1Hourly,
		},
		{
			name: "garbage next to an earlier backup is set aside under another name", file: []byte("not a gob stream"),
			oldBackup: []byte("earlier"), wantFiles: []string{"history.gob.20260930-120000.000.bak", "history.gob.bak"},
		},
		{
			name: "version 1 file next to an earlier backup leaves its copy under another name", file: v1,
			oldBackup: []byte("earlier"), wantFiles: []string{"history.gob", "history.gob.20260930-120000.000.bak", "history.gob.bak"},
			wantToday:  v1Today,
			wantCPU:    []float32{10, 20, 30},
			wantHourly: v1Hourly,
		},
		{
			name: "version 1 file next to its own copy leaves no second one", file: v1,
			oldBackup: v1, wantFiles: []string{"history.gob", "history.gob.bak"},
			wantToday:  v1Today,
			wantCPU:    []float32{10, 20, 30},
			wantHourly: v1Hourly,
		},
		{name: "read-only: a missing directory is not created", readOnly: true, wantFiles: []string{}},
		{
			name: "read-only: garbage stays in place and the backup is untouched", file: []byte("not a gob stream"),
			oldBackup: []byte("earlier"), readOnly: true, wantErr: ErrUndecodable, wantFiles: []string{"history.gob", "history.gob.bak"},
		},
		{
			name: "read-only: another version stays in place", file: future.Bytes(), readOnly: true, wantErr: ErrUndecodable,
			wantFiles: []string{"history.gob"},
		},
		{
			name: "read-only: version 1 file is read and not copied", file: v1, readOnly: true, wantFiles: []string{"history.gob"},
			wantToday:  v1Today,
			wantCPU:    []float32{10, 20, 30},
			wantHourly: v1Hourly,
		},
		{
			name: "file written before the hourly averages gets them from its minutes", file: noHourly.Bytes(),
			wantCPU:    []float32{40, 80},
			wantHourly: []minute{{At: 497436, N: 2, V: [seriesCount]float32{seriesCPU: 60}}},
		},
		{
			name: "file with hourly averages keeps them", file: withHourly.Bytes(),
			wantCPU:    []float32{40, 80},
			wantHourly: []minute{{At: 497000, N: 5, V: [seriesCount]float32{seriesCPU: 5}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The directory does not exist yet: Open creates it.
			dir := filepath.Join(t.TempDir(), "mac-pulse")
			if tt.file != nil {
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), tt.file, 0o600))
			}
			if tt.oldBackup != nil {
				require.NoError(t, os.WriteFile(filepath.Join(dir, fileName+".bak"), tt.oldBackup, 0o600))
			}
			noon := func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local) }

			var s *Store
			var err error
			if tt.readOnly {
				s, err = OpenReadOnly(dir)
			} else {
				require.NoError(t, os.MkdirAll(dir, 0o700))
				s, err = open(dir, false, noon)
			}

			require.ErrorIs(t, err, tt.wantErr)
			var gotCPU []float32
			for _, m := range s.st.Minutes {
				gotCPU = append(gotCPU, m.V[seriesCPU])
				assert.Zero(t, m.V[seriesPower], "a migrated minute has no power reading")
			}
			assert.Equal(t, tt.wantCPU, gotCPU)
			assert.Equal(t, tt.wantHourly, s.st.Hourly)
			assert.Equal(t, version, s.st.Version)
			s.now = noon
			assert.Equal(t, tt.wantToday, s.Today())
			// A loaded store must be writable: gob drops empty maps.
			s.AddUsage([]netinspect.AppUsage{{App: "curl", Down: 1}})
			s.Add(&collector.Snapshot{Time: s.now(), Apps: []collector.App{{Name: "curl"}}})
			if tt.wantFiles != nil {
				entries, _ := os.ReadDir(dir)
				names := []string{}
				for _, e := range entries {
					names = append(names, e.Name())
				}
				assert.Equal(t, tt.wantFiles, names)
				// Whatever Open did, the earlier backup and one copy of the file's bytes are still there.
				want := map[string][]byte{"history.gob.bak": tt.oldBackup, "history.gob.20260930-120000.000.bak": tt.file}
				if tt.readOnly {
					want["history.gob"] = tt.file
				}
				for _, name := range names {
					if content, ok := want[name]; ok {
						got, err := os.ReadFile(filepath.Join(dir, name))
						require.NoError(t, err)
						assert.Equal(t, content, got, name)
					}
				}
				return
			}
			if !tt.wantBackup {
				assert.NoFileExists(t, filepath.Join(dir, fileName+".bak"))
				return
			}
			backup, err := os.ReadFile(filepath.Join(dir, fileName+".bak"))
			require.NoError(t, err)
			assert.Equal(t, tt.file, backup)
			if tt.wantKept {
				assert.FileExists(t, filepath.Join(dir, fileName))
				return
			}
			assert.NoFileExists(t, filepath.Join(dir, fileName))
		})
	}
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestStore_Add(t *testing.T) {
	t.Parallel()

	evening := time.Date(2026, 9, 30, 23, 59, 56, 0, time.Local)
	// Ten busy minutes sampled every 2 s, as with the panel open, then fifty idle ones every 5 s.
	noon := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var busyHour []collector.Snapshot
	for at := noon; at.Before(noon.Add(10 * time.Minute)); at = at.Add(2 * time.Second) {
		busyHour = append(busyHour, collector.Snapshot{Time: at, CPU: collector.CPU{Total: 100}, Apps: []collector.App{{Name: "busy", CPU: 100}}})
	}
	for at := noon.Add(10 * time.Minute); at.Before(noon.Add(time.Hour)); at = at.Add(5 * time.Second) {
		busyHour = append(busyHour, collector.Snapshot{Time: at, Apps: []collector.App{{Name: "busy"}}})
	}
	system := 13.2
	overflow := make([]collector.Snapshot, sparkPoints+1)
	wantOverflow := make([]float64, sparkPoints)
	for i := range overflow {
		overflow[i] = collector.Snapshot{Time: evening.Add(-time.Hour), CPU: collector.CPU{Total: float64(i)}}
	}
	for i := range wantOverflow {
		wantOverflow[i] = float64(i + 1)
	}

	tests := []struct {
		name      string
		snaps     []collector.Snapshot
		wantToday Today
		// wantSpark lists the series that are not all zero.
		wantSpark Spark
		// wantHourCPU is the cpu percent of the hourly tier and of the app "busy", when not 0.
		wantHourCPU float64
	}{
		{
			name:        "hour weighs time, not samples: the faster sampling of an open panel counts no more",
			snaps:       busyHour,
			wantHourCPU: 100.0 / 6,
		},
		{
			name: "first reading is a baseline, later ones add their delta",
			snaps: []collector.Snapshot{
				{Time: evening.Add(-time.Hour), Disk: collector.Disk{WriteBytes: 1000}, Network: collector.Network{BytesRecv: 500, BytesSent: 50}},
				{
					Time:    evening.Add(-time.Hour + 2*time.Second),
					CPU:     collector.CPU{Total: 12.5},
					CPUTemp: 61,
					Memory:  collector.Memory{Total: 1000, Used: 250},
					GPU:     collector.GPU{Util: 40},
					Battery: collector.Battery{Power: -11.5},
					Disk:    collector.Disk{WriteBytes: 1300, ReadRate: 10, WriteRate: 150},
					Network: collector.Network{BytesRecv: 900, BytesSent: 60, DownRate: 200, UpRate: 5},
				},
			},
			wantToday: Today{DiskWritten: 300, NetDown: 400, NetUp: 10},
			wantSpark: Spark{
				CPU: []float64{0, 12.5}, Memory: []float64{0, 25}, GPU: []float64{0, 40}, Temp: []float64{0, 61}, Power: []float64{0, 11.5},
				DiskRead: []float64{0, 10}, DiskWrite: []float64{0, 150}, NetDown: []float64{0, 200}, NetUp: []float64{0, 5},
			},
		},
		{
			name: "local midnight restarts the counters",
			snaps: []collector.Snapshot{
				{Time: evening, Disk: collector.Disk{WriteBytes: 1000}, Network: collector.Network{BytesRecv: 1000, BytesSent: 1000}},
				{
					Time: evening.Add(2 * time.Second), Disk: collector.Disk{WriteBytes: 1400},
					Network: collector.Network{BytesRecv: 1700, BytesSent: 1010},
				},
				{
					Time: evening.Add(4 * time.Second), Disk: collector.Disk{WriteBytes: 1500},
					Network: collector.Network{BytesRecv: 1720, BytesSent: 1013},
				},
			},
			wantToday: Today{DiskWritten: 100, NetDown: 20, NetUp: 3},
		},
		{
			name: "failed collector and counter reset add nothing",
			snaps: []collector.Snapshot{
				{Time: evening.Add(-time.Hour), Disk: collector.Disk{WriteBytes: 1000}},
				{Time: evening.Add(-time.Hour), Disk: collector.Disk{}},
				{Time: evening.Add(-time.Hour), Disk: collector.Disk{WriteBytes: 1200}},
				{Time: evening.Add(-time.Hour), Disk: collector.Disk{WriteBytes: 50}},
				{Time: evening.Add(-time.Hour), Disk: collector.Disk{WriteBytes: 80}},
			},
			wantToday: Today{DiskWritten: 230},
		},
		{
			name: "power sparkline is the system draw when the SMC answers, and a non-finite reading plots as 0",
			snaps: []collector.Snapshot{
				{Time: evening.Add(-time.Hour), Battery: collector.Battery{Power: -11.5}, Power: collector.Power{SystemW: &system}},
				{Time: evening.Add(-time.Hour), CPU: collector.CPU{Total: math.NaN()}, CPUTemp: math.Inf(1)},
			},
			wantSpark: Spark{Power: []float64{13.2, 0}},
		},
		{
			name:      "sparkline keeps the newest 60 points",
			snaps:     overflow,
			wantSpark: Spark{CPU: wantOverflow},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			require.NoError(t, err)
			s.now = func() time.Time { return tt.snaps[len(tt.snaps)-1].Time }
			wantSpark := tt.wantSpark
			for _, series := range []*[]float64{
				&wantSpark.CPU, &wantSpark.Memory, &wantSpark.GPU, &wantSpark.Temp, &wantSpark.Power,
				&wantSpark.DiskRead, &wantSpark.DiskWrite, &wantSpark.NetDown, &wantSpark.NetUp,
			} {
				if *series == nil {
					*series = make([]float64, min(len(tt.snaps), sparkPoints))
				}
			}

			for i := range tt.snaps {
				s.Add(&tt.snaps[i])
			}

			assert.Equal(t, tt.wantToday, s.Today())
			assert.Equal(t, wantSpark, s.Spark())
			if tt.wantHourCPU != 0 {
				year, err := s.History("cpu", "1y")
				require.NoError(t, err)
				week, err := s.History("cpu", "7d")
				require.NoError(t, err)
				last := year.Series[0].Points[len(year.Series[0].Points)-1]
				require.NotNil(t, last)
				assert.InDelta(t, tt.wantHourCPU, *last, 0.1)
				require.Len(t, week.TopApps, 1)
				assert.InDelta(t, tt.wantHourCPU, week.TopApps[0].Value, 0.1)
			}
		})
	}
}

func TestStore_AddUsage(t *testing.T) {
	t.Parallel()

	evening := time.Date(2026, 9, 30, 23, 59, 30, 0, time.Local)
	type batch struct {
		at    time.Time
		usage []netinspect.AppUsage
	}

	tests := []struct {
		name    string
		batches []batch
		want    []AppTotal
	}{
		{name: "nothing recorded"},
		{
			name: "batches add up per app, busiest first",
			batches: []batch{
				{evening.Add(-time.Hour), []netinspect.AppUsage{{App: "curl", Down: 100, Up: 10}, {App: "Safari", Down: 5000, Up: 500}}},
				{evening.Add(-time.Hour + 30*time.Second), []netinspect.AppUsage{{App: "curl", Down: 50, Up: 5}}},
			},
			want: []AppTotal{{Name: "Safari", Down: 5000, Up: 500}, {Name: "curl", Down: 150, Up: 15}},
		},
		{
			name: "equal totals sort by name and the bundle path sticks",
			batches: []batch{
				{evening.Add(-time.Hour), []netinspect.AppUsage{{App: "b", Down: 10}, {App: "a", Up: 10, BundlePath: "/Applications/a.app"}}},
				{evening.Add(-time.Hour + 30*time.Second), []netinspect.AppUsage{{App: "a"}}},
			},
			want: []AppTotal{{Name: "a", BundlePath: "/Applications/a.app", Up: 10}, {Name: "b", Down: 10}},
		},
		{
			name: "local midnight drops yesterday's apps",
			batches: []batch{
				{evening, []netinspect.AppUsage{{App: "curl", Down: 100, Up: 10}}},
				{evening.Add(time.Minute), []netinspect.AppUsage{{App: "Safari", Down: 7, Up: 1}}},
			},
			want: []AppTotal{{Name: "Safari", Down: 7, Up: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			require.NoError(t, err)
			now := evening
			s.now = func() time.Time { return now }

			for _, b := range tt.batches {
				now = b.at
				s.AddUsage(b.usage)
			}

			assert.Equal(t, tt.want, s.Today().NetApps)
		})
	}
}

func TestStore_Flush(t *testing.T) {
	t.Parallel()

	noon := time.Date(2026, 9, 30, 12, 0, 30, 0, time.Local)

	tests := []struct {
		name  string
		snaps []collector.Snapshot
		usage []netinspect.AppUsage
	}{
		{name: "empty store"},
		{
			name: "samples, apps and traffic survive a reopen",
			snaps: []collector.Snapshot{
				{
					Time: noon.Add(-10 * time.Minute), CPU: collector.CPU{Total: 20}, Disk: collector.Disk{WriteBytes: 1000},
					Apps: []collector.App{{Name: "Xcode", BundlePath: "/Applications/Xcode.app", CPU: 150, RSS: 1 << 30}},
				},
				{
					Time: noon, CPU: collector.CPU{Total: 40}, Disk: collector.Disk{WriteBytes: 4000},
					Apps: []collector.App{{Name: "Xcode", BundlePath: "/Applications/Xcode.app", CPU: 50, RSS: 1 << 30}, {Name: "yes", CPU: 99}},
				},
			},
			usage: []netinspect.AppUsage{{App: "curl", Down: 100, Up: 10}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			s, err := Open(dir)
			require.NoError(t, err)
			s.now = func() time.Time { return noon }
			for i := range tt.snaps {
				s.Add(&tt.snaps[i])
			}
			s.AddUsage(tt.usage)

			require.NoError(t, s.Flush())
			reopened, err := Open(dir)

			require.NoError(t, err)
			reopened.now = s.now
			info, err := os.Stat(filepath.Join(dir, fileName))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			assert.NoFileExists(t, filepath.Join(dir, fileName+".tmp"))
			assert.Equal(t, s.Today(), reopened.Today())
			for _, metric := range []string{"cpu", "memory", "network"} {
				want, err := s.History(metric, "1h")
				require.NoError(t, err)
				got, err := reopened.History(metric, "1h")
				require.NoError(t, err)
				assert.Equal(t, want, got)
			}
			assert.Equal(t, Spark{}, reopened.Spark())
		})
	}
}

func TestStore_NetTotals(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 30, 12, 0, 30, 0, time.UTC)
	rate := func(ago time.Duration, down, up float64) collector.Snapshot {
		return collector.Snapshot{Time: now.Add(-ago), Network: collector.Network{DownRate: down, UpRate: up}}
	}

	tests := []struct {
		name  string
		snaps []collector.Snapshot
		want  NetTotals
	}{
		{name: "empty store"},
		{
			name:  "a minute counts 60 s at its average rate",
			snaps: []collector.Snapshot{rate(time.Minute, 100, 10), rate(time.Minute, 300, 30)},
			want:  NetTotals{Down7d: 12000, Up7d: 1200, Down30d: 12000, Up30d: 1200},
		},
		{
			name:  "a minute older than 7 days counts only towards the 30",
			snaps: []collector.Snapshot{rate(8*24*time.Hour, 50, 5), rate(6*24*time.Hour, 100, 10), rate(0, 1, 2)},
			want:  NetTotals{Down7d: 6060, Up7d: 720, Down30d: 9060, Up30d: 1020},
		},
		{
			name:  "a minute older than 30 days is gone",
			snaps: []collector.Snapshot{rate(31*24*time.Hour, 50, 5), rate(0, 1, 2)},
			want:  NetTotals{Down7d: 60, Up7d: 120, Down30d: 60, Up30d: 120},
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

			assert.Equal(t, tt.want, s.NetTotals())
		})
	}
}

// The closed alerts ride in history.gob. A file of a build that kept none opens without any, and
// a record written without a detail comes back with a nil one.
func TestStore_Alerts(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	full := alerts.Record{
		Alert: alerts.Alert{
			ID: "app_rulecpu:yes:5", Kind: alerts.KindAppRule, App: "yes", Since: at, Until: at.Add(time.Minute),
			Params: map[string]any{"app": "yes", "metric": "cpu", "value": 99.0, "limit": 50.0, "unit": "%", "minutes": 1},
		},
		Detail: &alerts.Detail{
			Unit: alerts.UnitPercent, Limit: 50, Hold: time.Minute, FiredAt: at.Add(time.Minute), Fired: 99, Peak: 100, Last: 3, Sum: 199, N: 2,
			Series: alerts.Series{Start: at.Unix(), Step: 10, V: []float32{99, 100}},
			Context: alerts.Context{
				PIDs: 1, TopUnit: alerts.UnitPercent, Top: []alerts.Row{{Name: "yes", Value: 99}},
				Procs: []alerts.Row{{Name: "yes", PID: 7, Value: 99}}, Facts: []alerts.Fact{{Key: "cpu_user", Value: 9, Unit: alerts.UnitPercent}},
			},
		},
	}
	bare := alerts.Record{Alert: alerts.Alert{
		ID: "disk::5", Kind: alerts.KindDisk, Since: at,
		Until: at.Add(time.Hour), Params: map[string]any{"limit": 10},
	}}
	// oldState is state as the build before alerts were kept wrote it; oldRecord a record before it had a detail.
	type oldState struct {
		Version int
		Minutes []minute
		Day     string
	}
	type oldRecord struct{ alerts.Alert }
	type bareState struct {
		Version int
		Alerts  []oldRecord
	}

	tests := []struct {
		name string
		// file is what history.gob holds; nil leaves the store to write it from set.
		file any
		set  []alerts.Record
		want []alerts.Record
	}{
		{name: "no alerts"},
		{name: "records survive a reopen, with and without a detail", set: []alerts.Record{full, bare}, want: []alerts.Record{full, bare}},
		{name: "file written before alerts were kept", file: oldState{Version: version, Minutes: []minute{{At: 5, N: 1}}, Day: "2026-09-30"}},
		{
			name: "record written without a detail",
			file: bareState{Version: version, Alerts: []oldRecord{{bare.Alert}}}, want: []alerts.Record{bare},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if tt.file != nil {
				var buf bytes.Buffer
				require.NoError(t, gob.NewEncoder(&buf).Encode(tt.file))
				require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), buf.Bytes(), 0o600))
			} else {
				s, err := Open(dir)
				require.NoError(t, err)
				s.SetAlerts(tt.set)
				require.NoError(t, s.Flush())
			}

			reopened, err := Open(dir)

			require.NoError(t, err)
			assert.Equal(t, tt.want, reopened.Alerts())
			assert.NoFileExists(t, filepath.Join(dir, fileName+".bak"), "the file must not be set aside as unreadable")
		})
	}
}
