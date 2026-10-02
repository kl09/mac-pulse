package collector

import (
	"runtime"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
)

func TestCPUShares(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prev       cpu.TimesStat
		cur        cpu.TimesStat
		wantUser   float64
		wantSystem float64
	}{
		{
			name:       "delta splits into user and system",
			prev:       cpu.TimesStat{User: 100, System: 50, Idle: 850},
			cur:        cpu.TimesStat{User: 110, System: 55, Idle: 935},
			wantUser:   10,
			wantSystem: 5,
		},
		{
			name:     "nice counts as user",
			prev:     cpu.TimesStat{User: 100, Nice: 10, System: 50, Idle: 840},
			cur:      cpu.TimesStat{User: 100, Nice: 35, System: 50, Idle: 915},
			wantUser: 25,
		},
		{
			name:       "zero previous reading averages since boot",
			cur:        cpu.TimesStat{User: 30, System: 20, Idle: 50},
			wantUser:   30,
			wantSystem: 20,
		},
		{
			name: "no time passed reads zero",
			prev: cpu.TimesStat{User: 100, System: 50, Idle: 850},
			cur:  cpu.TimesStat{User: 100, System: 50, Idle: 850},
		},
		{
			name: "counters going backwards read zero",
			prev: cpu.TimesStat{User: 100, System: 50, Idle: 850},
			cur:  cpu.TimesStat{User: 10, System: 5, Idle: 85},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user, system := cpuShares(tt.prev, tt.cur)

			assert.InDelta(t, tt.wantUser, user, 1e-9)
			assert.InDelta(t, tt.wantSystem, system, 1e-9)
		})
	}
}

func TestReadTopology(t *testing.T) {
	t.Parallel()

	model, clusters := readTopology()

	assert.NotEmpty(t, model)
	// An Intel Mac has no performance levels; wherever they exist they cover every core,
	// efficiency cluster first when there is more than one.
	if len(clusters) == 0 {
		return
	}
	cores := 0
	for _, c := range clusters {
		assert.NotEmpty(t, c.Name)
		cores += c.Cores
	}
	assert.Equal(t, runtime.NumCPU(), cores)
	// A virtual Mac reports a single level called "Standard".
	if len(clusters) > 1 {
		assert.Equal(t, "Performance", clusters[len(clusters)-1].Name)
	}
}
