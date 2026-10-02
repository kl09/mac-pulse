package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/store"
	"github.com/kl09/mac-pulse/internal/ui"
)

func TestViewNeeds(t *testing.T) {
	t.Parallel()

	both := map[string]bool{"popover": true, "window": true}
	tests := []struct {
		name        string
		tabs        map[string]string
		visible     map[string]bool
		wantNet     bool
		wantResolve bool
		wantAllApps bool
		wantStorage bool
		wantDetail  collector.Detail
	}{
		{name: "no view has reported a tab yet", visible: both},
		{name: "overview needs nothing", tabs: map[string]string{"popover": "overview"}, visible: both},
		{name: "apps tab lists every app", tabs: map[string]string{"window": "processes"}, visible: both, wantAllApps: true},
		{
			name: "network tab runs the inspector with reverse DNS", tabs: map[string]string{"popover": "network"}, visible: both,
			wantNet: true, wantResolve: true,
		},
		{
			name: "network detail runs the inspector without reverse DNS", tabs: map[string]string{"popover": "detail:network"}, visible: both,
			wantNet: true, wantDetail: collector.Detail{NetInfo: true},
		},
		{
			name: "cpu detail reads the sensors", tabs: map[string]string{"popover": "detail:cpu"}, visible: both,
			wantDetail: collector.Detail{Temps: true},
		},
		{
			name: "gpu detail reads the sensors", tabs: map[string]string{"window": "detail:gpu"}, visible: both,
			wantDetail: collector.Detail{Temps: true},
		},
		{
			name: "sensors detail adds Bluetooth", tabs: map[string]string{"popover": "detail:sensors"}, visible: both,
			wantDetail: collector.Detail{Temps: true, Bluetooth: true},
		},
		{
			name: "disk detail reads SMART and, for its cleanup block, gets the storage part", tabs: map[string]string{"popover": "detail:disk"},
			visible: both, wantStorage: true, wantDetail: collector.Detail{SMART: true},
		},
		{name: "storage tab gets the storage part", tabs: map[string]string{"window": "storage"}, visible: both, wantStorage: true},
		{
			name: "a hidden storage tab gets no storage part",
			tabs: map[string]string{"popover": "overview", "window": "storage"}, visible: map[string]bool{"popover": true},
		},
		{
			name: "dev tab collects agents and containers, and no network report", tabs: map[string]string{"popover": "dev"}, visible: both,
			wantDetail: collector.Detail{Dev: true},
		},
		{
			name: "dev next to the network tab",
			tabs: map[string]string{"popover": "dev", "window": "network"}, visible: both,
			wantNet: true, wantResolve: true, wantDetail: collector.Detail{Dev: true},
		},
		{
			name: "memory and battery details need nothing",
			tabs: map[string]string{"popover": "detail:memory", "window": "detail:battery"}, visible: both,
		},
		{
			name: "a hidden view asks for nothing, whatever it was left on",
			tabs: map[string]string{"popover": "detail:sensors", "window": "processes"}, visible: map[string]bool{"popover": false},
		},
		{
			name: "only the visible view counts",
			tabs: map[string]string{"popover": "detail:sensors", "window": "detail:disk"}, visible: map[string]bool{"window": true},
			wantStorage: true, wantDetail: collector.Detail{SMART: true},
		},
		{
			name: "a hidden network tab does not keep reverse DNS on",
			tabs: map[string]string{"popover": "detail:network", "window": "network"}, visible: map[string]bool{"popover": true},
			wantNet: true, wantDetail: collector.Detail{NetInfo: true},
		},
		{
			name: "two visible views add up",
			tabs: map[string]string{"popover": "detail:network", "window": "detail:sensors"}, visible: both,
			wantNet: true, wantDetail: collector.Detail{Temps: true, Bluetooth: true, NetInfo: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			net, resolve, allApps, storage, detail := viewNeeds(tt.tabs, tt.visible)

			assert.Equal(t, tt.wantNet, net)
			assert.Equal(t, tt.wantResolve, resolve)
			assert.Equal(t, tt.wantAllApps, allApps)
			assert.Equal(t, tt.wantStorage, storage)
			assert.Equal(t, tt.wantDetail, detail)
		})
	}
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// taken is the suffix of a file that already exists under the base name.
		taken   string
		wantErr bool
		// wantFiles are the names left in the folder.
		wantFiles []string
	}{
		{name: "both files are created", wantFiles: []string{"out-apps.csv", "out-system.csv"}},
		{name: "an existing apps file is kept, no system file is left", taken: "-apps.csv", wantErr: true, wantFiles: []string{"out-apps.csv"}},
		{name: "an existing system file is kept, no apps file made", taken: "-system.csv", wantErr: true, wantFiles: []string{"out-system.csv"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			history, err := store.Open(filepath.Join(dir, "data"))
			require.NoError(t, err)
			folder := filepath.Join(dir, "export")
			require.NoError(t, os.Mkdir(folder, 0o755))
			if tt.taken != "" {
				require.NoError(t, os.WriteFile(filepath.Join(folder, "out"+tt.taken), []byte("mine"), 0o644))
			}

			err = writeCSV(history, filepath.Join(folder, "out"))

			assert.Equal(t, tt.wantErr, err != nil, "%v", err)
			entries, readErr := os.ReadDir(folder)
			require.NoError(t, readErr)
			names := []string{}
			for _, e := range entries {
				names = append(names, e.Name())
			}
			assert.Equal(t, tt.wantFiles, names)
			if tt.taken != "" {
				kept, err := os.ReadFile(filepath.Join(folder, "out"+tt.taken))
				require.NoError(t, err)
				assert.Equal(t, "mine", string(kept))
				return
			}
			system, err := os.ReadFile(filepath.Join(folder, "out-system.csv"))
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(string(system), "time,cpu_percent,"), "the system file starts with its header")
		})
	}
}

func TestSession_group(t *testing.T) {
	t.Parallel()

	sampled := &session{latest: &latest{}}
	sampled.latest.update(func(c *current) {
		c.snap = &collector.Snapshot{Apps: []collector.App{
			{Name: "iTerm", BundlePath: "/Applications/iTerm.app", PIDs: []int32{50, 51}},
			{Name: "curl", PIDs: []int32{60}},
		}}
	})
	tests := []struct {
		name       string
		sess       *session
		pid        int32
		wantName   string
		wantBundle string
		wantOK     bool
	}{
		{
			name: "a shell goes by the terminal the Apps tab puts it in", sess: sampled, pid: 51,
			wantName: "iTerm", wantBundle: "/Applications/iTerm.app", wantOK: true,
		},
		{name: "a group outside any bundle", sess: sampled, pid: 60, wantName: "curl", wantOK: true},
		{name: "a pid the sample has not seen is left to the collector", sess: sampled, pid: 4242},
		{name: "before the first sample", sess: &session{latest: &latest{}}, pid: 51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			name, bundle, ok := tt.sess.group(tt.pid)

			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantBundle, bundle)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestStatusOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		menuBar []string
		want    []string
	}{
		{name: "the default menu bar leads, the rest follow to be taken away", menuBar: []string{"cpu", "mem"}, want: statusKeys},
		{
			name: "the items are made in the order of the Settings list", menuBar: []string{"disk", "temp", "cpu"},
			want: []string{"disk", "temp", "cpu", "mem", "net", "battery"},
		},
		{
			name: "every key in the menu bar, reversed", menuBar: []string{"disk", "battery", "temp", "net", "mem", "cpu"},
			want: []string{"disk", "battery", "temp", "net", "mem", "cpu"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, statusOrder(tt.menuBar))
		})
	}
}

func TestEncodeState(t *testing.T) {
	t.Parallel()

	alert := func(value float64) ui.Alerts {
		return ui.Alerts{Active: []ui.Alert{{ID: "cpu::5", Kind: "cpu", Params: map[string]any{"value": value}}}, Recent: []ui.Alert{}}
	}
	tests := []struct {
		name string
		in   ui.Alerts
		want ui.Alerts
	}{
		{name: "a state that encodes keeps its alerts", in: alert(95), want: alert(95)},
		{
			name: "an alert with a number JSON cannot say: the state goes out without the alerts",
			in:   alert(math.NaN()), want: ui.Alerts{Active: []ui.Alert{}, Recent: []ui.Alert{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw, err := encodeState(ui.State{Time: 7, Alerts: tt.in})

			require.NoError(t, err)
			var got ui.State
			require.NoError(t, json.Unmarshal(raw, &got))
			assert.Equal(t, int64(7), got.Time, "the rest of the state is there")
			assert.Equal(t, tt.want, got.Alerts)
		})
	}
}
