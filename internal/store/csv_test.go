package store

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
)

func TestStore_WriteCSV(t *testing.T) {
	t.Parallel()

	const systemHeader = "time,cpu_percent,memory_bytes,net_down_bytes_per_s,net_up_bytes_per_s,disk_read_bytes_per_s," +
		"disk_write_bytes_per_s,gpu_percent,temp_c,battery_percent,power_w,cpu_w,gpu_w\n"
	const appsHeader = "date,app,cpu_percent_avg,memory_bytes_avg,network_bytes\n"
	// Local, so that the two samples fall on two local dates in any time zone.
	evening := time.Date(2026, 9, 29, 22, 10, 0, 0, time.Local)
	noon := time.Date(2026, 9, 30, 12, 10, 0, 0, time.Local)
	hour := func(at time.Time) string { return at.Truncate(time.Hour).UTC().Format(time.RFC3339) }
	watts := 12.345

	tests := []struct {
		name  string
		snaps []collector.Snapshot
		usage []netinspect.AppUsage
		// closed writes the system document to a file that is already closed.
		closed     bool
		wantSystem string
		wantApps   string
		wantErr    bool
	}{
		{name: "empty store writes the two headers", wantSystem: systemHeader, wantApps: appsHeader},
		{
			name: "a row per hour and per app and local day; a series without a reading leaves its cell empty; a formula-like name is quoted",
			snaps: []collector.Snapshot{
				{
					Time: evening, CPU: collector.CPU{Total: 10}, Memory: collector.Memory{Used: 8 << 30},
					Apps: []collector.App{{Name: "Xcode", CPU: 150, RSS: 2 << 30}, {Name: "idle"}},
				},
				{
					Time: evening.Add(2 * time.Second), CPU: collector.CPU{Total: 20.5}, Memory: collector.Memory{Used: 8 << 30},
					Apps: []collector.App{{Name: "Xcode", CPU: 50.5, RSS: 4 << 30}},
				},
				{
					Time: noon, CPU: collector.CPU{Total: 33.33}, CPUTemp: 61.25, GPU: collector.GPU{Util: 40},
					Network: collector.Network{DownRate: 2000.4, UpRate: 100}, Disk: collector.Disk{ReadRate: 10, WriteRate: 150},
					Battery: collector.Battery{Percent: 78}, Power: collector.Power{SystemW: &watts},
					Apps: []collector.App{{Name: "Tom, Dick & Harry", CPU: 7}},
				},
			},
			usage: []netinspect.AppUsage{{App: "curl", Down: 100, Up: 10}, {App: "=HYPERLINK(\"http://x\")", Down: 1}},
			wantSystem: systemHeader +
				hour(evening) + ",15.2,8589934592,0,0,0,0,0.0,,0,,,\n" +
				hour(noon) + ",33.3,0,2000,100,10,150,40.0,61.2,78,12.35,,\n",
			wantApps: appsHeader +
				"2026-09-29,Xcode,83.7,3579139413,0\n" +
				"2026-09-30,\"'=HYPERLINK(\"\"http://x\"\")\",0.0,0,1\n" +
				"2026-09-30,\"Tom, Dick & Harry\",7.0,0,0\n" +
				"2026-09-30,curl,0.0,0,110\n",
		},
		{name: "a writer that fails is an error", closed: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			require.NoError(t, err)
			s.now = func() time.Time { return noon }
			for i := range tt.snaps {
				s.Add(&tt.snaps[i])
			}
			s.AddUsage(tt.usage)
			var system, apps strings.Builder
			closed, err := os.CreateTemp(t.TempDir(), "")
			require.NoError(t, err)
			require.NoError(t, closed.Close())

			if tt.closed {
				err = s.WriteCSV(closed, &apps)
			} else {
				err = s.WriteCSV(&system, &apps)
			}

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSystem, system.String())
			assert.Equal(t, tt.wantApps, apps.String())
		})
	}
}
