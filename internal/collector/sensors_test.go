package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSensorGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key      string
		wantRank int
		wantName string
	}{
		{key: "TPD0", wantRank: 0, wantName: "Power management chip"},
		{key: "TPDX", wantRank: 0, wantName: "Power management chip"},
		{key: "Tp01", wantRank: 1, wantName: "CPU performance"},
		{key: "Tp2X", wantRank: 1, wantName: "CPU performance"},
		{key: "Te04", wantRank: 2, wantName: "CPU efficiency"},
		{key: "Tg1l", wantRank: 3, wantName: "GPU"},
		{key: "TB0T", wantRank: 4, wantName: "Battery"},
		{key: "TaLP", wantRank: 5, wantName: "Airflow"},
		{key: "Ts0P", wantRank: 6, wantName: "Palm rest"},
		{key: "TW0P", wantRank: 7, wantName: "Wireless"},
		{key: "TCMz", wantRank: 8, wantName: "Other"},
		{key: "TP0b", wantRank: 8, wantName: "Other"},
		{key: "", wantRank: 8, wantName: "Other"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			rank, name := sensorGroup(tt.key)

			assert.Equal(t, tt.wantRank, rank)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

func TestListTemps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		temps map[string]float64
		want  []Sensor
	}{
		{name: "no keys", want: []Sensor{}},
		{
			name:  "groups in display order, keys in order inside a group",
			temps: map[string]float64{"TCMz": 51, "Te04": 44.5, "Tp05": 61, "Tp01": 58.2, "TPD3": 47},
			want: []Sensor{
				{Group: "Power management chip", Key: "TPD3", Temp: 47},
				{Group: "CPU performance", Key: "Tp01", Temp: 58.2},
				{Group: "CPU performance", Key: "Tp05", Temp: 61},
				{Group: "CPU efficiency", Key: "Te04", Temp: 44.5},
				{Group: "Other", Key: "TCMz", Temp: 51},
			},
		},
		{
			name:  "a key below the floor is not a temperature",
			temps: map[string]float64{"Tp01": 3.2, "Tp05": 9.99, "Tp09": 10},
			want:  []Sensor{{Group: "CPU performance", Key: "Tp09", Temp: 10}},
		},
		{
			name:  "exactly 40.0 is the placeholder of an idle key",
			temps: map[string]float64{"Tp01": 40, "Tp05": 40.0, "Tp09": 40.1, "Te04": 39.9},
			want: []Sensor{
				{Group: "CPU performance", Key: "Tp09", Temp: 40.1},
				{Group: "CPU efficiency", Key: "Te04", Temp: 39.9},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, listTemps(tt.temps))
		})
	}
}
