package collector

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBattery(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/ioreg_battery.txt")
	require.NoError(t, err)
	full := Battery{
		Present:           true,
		Percent:           100,
		IsCharging:        true,
		ExternalConnected: true,
		TimeRemaining:     0,
		CycleCount:        119,
		Health:            93,
		Temperature:       30.81,
		Voltage:           13232,
		Amperage:          856,
		Power:             13232 * 856 / 1e6,
		DesignMAh:         6249,
		MaxMAh:            5830,
		Adapter:           &Adapter{Name: "70W USB-C Power Adapter", Watts: 68},
	}
	unplugged := full
	unplugged.ExternalConnected, unplugged.Adapter = false, nil
	nameless := full
	nameless.Adapter = &Adapter{Watts: 68}
	discharging := full
	discharging.Amperage = -32
	discharging.Power = 13232 * -32 / 1e6

	const nominalLine = "\n      \"NominalChargeCapacity\" = 6031"
	nominal := full
	nominal.MaxMAh, nominal.Health = 6031, 96

	tests := []struct {
		name    string
		input   string
		want    Battery
		wantErr bool
	}{
		{name: "full fixture", input: string(fixture), want: full},
		{
			name:  "negative amperage wrapped as uint64",
			input: strings.Replace(string(fixture), `"InstantAmperage" = 856`, `"InstantAmperage" = 18446744073709551584`, 1),
			want:  discharging,
		},
		{
			name:  "no-estimate sentinel reads zero",
			input: strings.Replace(string(fixture), `"TimeRemaining" = 0`, `"TimeRemaining" = 65535`, 1),
			want:  full,
		},
		{
			name:  "on battery a stale adapter dictionary is not an adapter",
			input: strings.Replace(string(fixture), `"ExternalConnected" = Yes`, `"ExternalConnected" = No`, 1),
			want:  unplugged,
		},
		{
			name:  "adapter dictionary without watts is no adapter",
			input: strings.Replace(string(fixture), `"Watts"=68,`, "", 1),
			want:  func() Battery { b := full; b.Adapter = nil; return b }(),
		},
		{
			name:  "adapter without a name keeps its watts",
			input: strings.Replace(string(fixture), `"Name"="70W USB-C Power Adapter ",`, "", 1),
			want:  nameless,
		},
		{
			name:  "nominal capacity wins over the raw one",
			input: strings.Replace(string(fixture), `"DesignCapacity" = 6249`, `"DesignCapacity" = 6249`+nominalLine, 1),
			want:  nominal,
		},
		{name: "empty input", input: "", wantErr: true},
		{
			name:    "missing key",
			input:   strings.Replace(string(fixture), `"CycleCount" = 119`, "", 1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBattery(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
