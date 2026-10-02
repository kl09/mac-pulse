package collector

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBluetooth(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/system_profiler_bluetooth.json")
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    []BluetoothDevice
		wantErr bool
	}{
		{
			name:  "fixture: connected devices with a battery; the headset without one and the disconnected phone are skipped",
			input: string(fixture),
			want: []BluetoothDevice{
				{
					Name: "Alex\u2019s AirPods", Kind: "Headphones",
					Levels: []BatteryLevel{{Part: "left", Percent: 99}, {Part: "right", Percent: 95}, {Part: "case", Percent: 64}},
				},
				{Name: "Keyboard K380", Kind: "Keyboard", Levels: []BatteryLevel{{Part: "main", Percent: 70}}},
			},
		},
		{
			name:  "level out of range is clamped, garbage is skipped, a missing type reads empty",
			input: `{"SPBluetoothDataType":[{"device_connected":[{"Mouse":{"device_batteryLevelMain":"140 %","device_batteryLevelCase":"n/a"}}]}]}`,
			want:  []BluetoothDevice{{Name: "Mouse", Levels: []BatteryLevel{{Part: "main", Percent: 100}}}},
		},
		{
			name:  "value that is not a string is not a level",
			input: `{"SPBluetoothDataType":[{"device_connected":[{"Mouse":{"device_batteryLevelMain":70,"device_minorType":["x"]}}]}]}`,
		},
		{name: "bluetooth off: no device lists", input: `{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_off"}}]}`},
		{name: "no controller", input: `{"SPBluetoothDataType":[]}`},
		{name: "not JSON", input: "Bluetooth:\n", wantErr: true},
		{name: "empty input", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBluetooth(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
