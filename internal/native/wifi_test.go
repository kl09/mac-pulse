package native

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWiFiInfo(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	wifi, err := WiFiInfo()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("Wi-Fi is off or not associated")
	}
	require.NoError(t, err)

	assert.NotEmpty(t, wifi.Interface)
	assert.Negative(t, wifi.RSSI)
	assert.Positive(t, wifi.Channel)
	assert.Contains(t, []float64{2.4, 5, 6}, wifi.BandGHz)
}

func TestThermalState(t *testing.T) {
	t.Parallel()

	state := ThermalState()

	assert.GreaterOrEqual(t, state, 0)
	assert.LessOrEqual(t, state, 3)
}

func TestPHYName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode int
		want string
	}{
		{name: "none", mode: 0, want: ""},
		{name: "Wi-Fi 4", mode: 4, want: "802.11n"},
		{name: "Wi-Fi 6", mode: 6, want: "802.11ax"},
		{name: "newer than the table", mode: 99, want: ""},
		{name: "negative", mode: -1, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, phyName(tt.mode))
		})
	}
}

func TestSecurityName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		security int
		want     string
	}{
		{name: "open network", security: 0, want: "None"},
		{name: "WPA2 Personal", security: 4, want: "WPA2 Personal"},
		{name: "WPA3 Personal", security: 11, want: "WPA3 Personal"},
		{name: "last known", security: 15, want: "OWE Transition"},
		{name: "kCWSecurityUnknown is NSIntegerMax", security: 1<<63 - 1, want: ""},
		{name: "negative", security: -1, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, securityName(tt.security))
		})
	}
}

func TestChannelWidthMHz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		width int
		want  int
	}{
		{name: "unknown", width: 0, want: 0},
		{name: "20 MHz", width: 1, want: 20},
		{name: "160 MHz", width: 4, want: 160},
		{name: "newer than the table", width: 5, want: 0},
		{name: "negative", width: -1, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, channelWidthMHz(tt.width))
		})
	}
}

func TestBandGHz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		band int
		want float64
	}{
		{name: "unknown", band: 0, want: 0},
		{name: "2.4 GHz", band: 1, want: 2.4},
		{name: "6 GHz", band: 3, want: 6},
		{name: "newer than the table", band: 4, want: 0},
		{name: "negative", band: -1, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, bandGHz(tt.band), 0)
		})
	}
}
