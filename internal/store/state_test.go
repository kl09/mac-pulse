package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestState_addSample(t *testing.T) {
	t.Parallel()

	noon := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	at, hr := noon.Unix()/60, noon.Unix()/3600
	type sample struct {
		at  time.Time
		cpu float32
	}

	tests := []struct {
		name string
		// start is what the file held.
		start      []minute
		samples    []sample
		want       []minute
		wantHourly []minute
	}{
		{
			name:       "samples of one minute average, and the minute counts once in its hour",
			samples:    []sample{{noon, 10}, {noon.Add(2 * time.Second), 30}, {noon.Add(58 * time.Second), 50}},
			want:       []minute{{At: at, N: 3, V: [seriesCount]float32{seriesCPU: 30}}},
			wantHourly: []minute{{At: hr, N: 1, V: [seriesCount]float32{seriesCPU: 30}}},
		},
		{
			name:    "minute without samples leaves a gap, and the hour averages minutes, not samples",
			samples: []sample{{noon, 10}, {noon.Add(time.Second), 10}, {noon.Add(2 * time.Second), 10}, {noon.Add(2 * time.Minute), 50}},
			want: []minute{
				{At: at, N: 3, V: [seriesCount]float32{seriesCPU: 10}},
				{At: at + 2, N: 1, V: [seriesCount]float32{seriesCPU: 50}},
			},
			wantHourly: []minute{{At: hr, N: 2, V: [seriesCount]float32{seriesCPU: 30}}},
		},
		{
			name:    "minutes older than 30 days are dropped, their hour stays",
			samples: []sample{{noon.Add(-30 * 24 * time.Hour), 10}, {noon.Add(-30*24*time.Hour + time.Minute), 20}, {noon, 40}},
			want: []minute{
				{At: at - retainMinutes + 1, N: 1, V: [seriesCount]float32{seriesCPU: 20}},
				{At: at, N: 1, V: [seriesCount]float32{seriesCPU: 40}},
			},
			wantHourly: []minute{
				{At: hr - 30*24, N: 2, V: [seriesCount]float32{seriesCPU: 15}},
				{At: hr, N: 1, V: [seriesCount]float32{seriesCPU: 40}},
			},
		},
		{
			name:    "hours older than 366 days are dropped",
			samples: []sample{{noon.Add(-366 * 24 * time.Hour), 10}, {noon.Add(-366*24*time.Hour + time.Hour), 20}, {noon, 40}},
			want:    []minute{{At: at, N: 1, V: [seriesCount]float32{seriesCPU: 40}}},
			wantHourly: []minute{
				{At: hr - retainHourly + 1, N: 1, V: [seriesCount]float32{seriesCPU: 20}},
				{At: hr, N: 1, V: [seriesCount]float32{seriesCPU: 40}},
			},
		},
		{
			name:       "clock stepping back drops the minutes and the hours after it",
			samples:    []sample{{noon, 10}, {noon.Add(-5 * time.Minute), 90}},
			want:       []minute{{At: at - 5, N: 1, V: [seriesCount]float32{seriesCPU: 90}}},
			wantHourly: []minute{{At: hr - 1, N: 1, V: [seriesCount]float32{seriesCPU: 90}}},
		},
		{
			name:    "forward jump then correction keeps recording",
			samples: []sample{{noon, 10}, {noon.Add(10 * time.Minute), 20}, {noon.Add(time.Minute), 30}, {noon.Add(2 * time.Minute), 40}},
			want: []minute{
				{At: at, N: 1, V: [seriesCount]float32{seriesCPU: 10}},
				{At: at + 1, N: 1, V: [seriesCount]float32{seriesCPU: 30}},
				{At: at + 2, N: 1, V: [seriesCount]float32{seriesCPU: 40}},
			},
			wantHourly: []minute{{At: hr, N: 4, V: [seriesCount]float32{seriesCPU: 25}}},
		},
		{
			name:    "damaged count in the file neither poisons its minute nor reaches the hour",
			start:   []minute{{At: at - 1, N: 0, V: [seriesCount]float32{seriesCPU: 999}}, {At: at, N: -4, V: [seriesCount]float32{seriesCPU: 999}}},
			samples: []sample{{noon, 30}},
			want: []minute{
				{At: at - 1, N: 0, V: [seriesCount]float32{seriesCPU: 999}},
				{At: at, N: 1, V: [seriesCount]float32{seriesCPU: 30}},
			},
			wantHourly: []minute{{At: hr, N: 1, V: [seriesCount]float32{seriesCPU: 30}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := state{Minutes: tt.start}
			st.fillHourly()

			for _, s := range tt.samples {
				st.addSample(s.at, [seriesCount]float32{seriesCPU: s.cpu})
			}

			assert.Equal(t, tt.want, st.Minutes)
			assert.Equal(t, tt.wantHourly, st.Hourly)
		})
	}
}

func TestState_fillHourly(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC).Unix() / 60
	minutes := []minute{
		{At: at - 61, N: 2, V: [seriesCount]float32{seriesCPU: 70}},
		{At: at, N: 1, V: [seriesCount]float32{seriesCPU: 10}},
		{At: at + 1, N: 5, V: [seriesCount]float32{seriesCPU: 30}},
		{At: at + 2, N: 0, V: [seriesCount]float32{seriesCPU: 999}},
		{At: at + 59, N: 3, V: [seriesCount]float32{seriesCPU: 50}},
	}

	tests := []struct {
		name   string
		hourly []minute
		want   []minute
	}{
		{
			name: "no hours yet: each hour averages its minutes, one weight each, without the damaged one",
			want: []minute{
				{At: at/60 - 2, N: 1, V: [seriesCount]float32{seriesCPU: 70}},
				{At: at / 60, N: 3, V: [seriesCount]float32{seriesCPU: 30}},
			},
		},
		{
			name:   "hours already there are left alone",
			hourly: []minute{{At: at/60 - 500, N: 9, V: [seriesCount]float32{seriesCPU: 1}}},
			want:   []minute{{At: at/60 - 500, N: 9, V: [seriesCount]float32{seriesCPU: 1}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := state{Minutes: minutes, Hourly: tt.hourly}

			st.fillHourly()

			assert.Equal(t, tt.want, st.Hourly)
		})
	}
}

func TestState_openHour(t *testing.T) {
	t.Parallel()

	noon := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	at := noon.Unix() / 3600

	tests := []struct {
		name  string
		hours []hour
		now   time.Time
		want  []hour
	}{
		{
			name: "first hour",
			now:  noon,
			want: []hour{{At: at, Apps: map[string]appHour{}}},
		},
		{
			name:  "same hour is reused untouched",
			hours: []hour{{At: at, Samples: 3, Apps: map[string]appHour{"idle": {}}}},
			now:   noon.Add(59 * time.Minute),
			want:  []hour{{At: at, Samples: 3, Apps: map[string]appHour{"idle": {}}}},
		},
		{
			name:  "new hour closes and prunes the previous one",
			hours: []hour{{At: at - 1, Samples: 3, Apps: map[string]appHour{"idle": {}, "busy": {CPU: 5}}}},
			now:   noon,
			want: []hour{
				{At: at - 1, Samples: 3, Apps: map[string]appHour{"busy": {CPU: 5}}},
				{At: at, Apps: map[string]appHour{}},
			},
		},
		{
			name:  "hours older than 30 days are dropped",
			hours: []hour{{At: at - retainHours, Apps: map[string]appHour{}}, {At: at - retainHours + 1, Apps: map[string]appHour{}}},
			now:   noon,
			want:  []hour{{At: at - retainHours + 1, Apps: map[string]appHour{}}, {At: at, Apps: map[string]appHour{}}},
		},
		{
			name:  "clock stepping back drops the hours after it",
			hours: []hour{{At: at - 4, Samples: 2, Apps: map[string]appHour{}}, {At: at, Samples: 1, Apps: map[string]appHour{"busy": {CPU: 5}}}},
			now:   noon.Add(-3 * time.Hour),
			want:  []hour{{At: at - 4, Samples: 2, Apps: map[string]appHour{}}, {At: at - 3, Apps: map[string]appHour{}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := state{Hours: tt.hours}

			got := st.openHour(tt.now)

			assert.Equal(t, tt.want, st.Hours)
			assert.Same(t, &st.Hours[len(st.Hours)-1], got)
		})
	}
}

func TestHour_prune(t *testing.T) {
	t.Parallel()

	// cpu00 is the busiest of twelve; their memory and network stay zero.
	crowd := map[string]appHour{}
	for i := range 12 {
		crowd[fmt.Sprintf("cpu%02d", i)] = appHour{CPU: float64(100 - i)}
	}
	crowdTop := map[string]appHour{}
	for i := range topAppsPerHour {
		crowdTop[fmt.Sprintf("cpu%02d", i)] = appHour{CPU: float64(100 - i)}
	}
	withOthers := map[string]appHour{"ram": {Memory: 1 << 30}, "net": {Net: 1 << 20}, "idle": {}}
	wantWithOthers := map[string]appHour{"ram": {Memory: 1 << 30}, "net": {Net: 1 << 20}}
	for name, a := range crowd {
		withOthers[name] = a
	}
	for name, a := range crowdTop {
		wantWithOthers[name] = a
	}

	tests := []struct {
		name string
		apps map[string]appHour
		want map[string]appHour
	}{
		{name: "empty hour", apps: map[string]appHour{}, want: map[string]appHour{}},
		{
			name: "idle apps go even when the hour is small",
			apps: map[string]appHour{"busy": {CPU: 5}, "idle": {}},
			want: map[string]appHour{"busy": {CPU: 5}},
		},
		{name: "only the top ten by cpu stay", apps: crowd, want: crowdTop},
		{name: "top by memory and by network stay next to the cpu top", apps: withOthers, want: wantWithOthers},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hour{Apps: tt.apps}

			h.prune()

			assert.Equal(t, tt.want, h.Apps)
		})
	}
}
