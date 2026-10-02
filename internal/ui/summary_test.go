package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSummary(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 1, 12, 30, 0, 0, time.Local)
	temp, watts := 63.4, 13.24
	tests := []struct {
		name string
		in   State
		want string
	}{
		{
			name: "a desktop before the first process scan has no battery, temperature, power or apps line",
			in: State{
				Time:   at.UnixMilli(),
				Memory: Memory{Pressure: "normal"},
				Apps:   Apps{Items: []App{{Name: "Finder"}}},
			},
			want: "mac-pulse · 2026-10-01 12:30\n" +
				"CPU: 0% (user 0%, system 0%), load 0.00 0.00 0.00\n" +
				"Memory: 0 B of 0 B used, pressure normal\n" +
				"Disk: 0 B free of 0 B\n" +
				"Network: down 0 B/s, up 0 B/s\n",
		},
		{
			name: "every line, the three busiest apps, the temperature in the chosen unit",
			in: State{
				Time:    at.UnixMilli(),
				CPU:     CPU{Total: 37.2, User: 24.9, System: 12.3, Load: [3]float64{4.21, 3.5, 3}, Temp: &temp},
				Memory:  Memory{Total: 24 << 30, Used: 15253999616, Pressure: "warning"},
				Disk:    Disk{Total: 994662584320, Free: 227633266688},
				Network: Network{DownRate: 4508876.8, UpRate: 999.6},
				Battery: &Battery{Percent: 78, State: "battery", Health: 93, Cycles: 119},
				Power:   Power{System: &watts},
				Sensors: Sensors{Thermal: "fair"},
				Apps: Apps{HasRates: true, Items: []App{
					{Name: "Google Chrome", CPU: 181.5, Memory: 4349493248},
					{Name: "Xcode", CPU: 40, Memory: 900 << 20},
					{Name: "yes", CPU: 12.34, Memory: 512},
					{Name: "Finder", CPU: 1},
				}},
				Settings: Settings{TempUnit: "F"},
			},
			want: "mac-pulse · 2026-10-01 12:30\n" +
				"CPU: 37% (user 25%, system 12%), load 4.21 3.50 3.00\n" +
				"Memory: 14.2 GB of 24.0 GB used, pressure warning\n" +
				"Disk: 228 GB free of 995 GB\n" +
				"Network: down 4.3 MB/s, up 1.0 KB/s\n" +
				"Battery: 78% (battery), health 93%, 119 cycles\n" +
				"Temperature: 146 °F, thermal state fair\n" +
				"Power: 13.2 W\n" +
				"Top apps (CPU / memory): Google Chrome 181.5% / 4.1 GB, Xcode 40.0% / 900 MB, yes 12.3% / 512 B\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Summary(tt.in))
		})
	}
}
