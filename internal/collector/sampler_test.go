package collector

import (
	"cmp"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kl09/mac-pulse/internal/native"
)

func TestSampler_Run(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("samples this Mac: sensors, disks, Bluetooth, the network")
	}

	// failsafe ends a run whose sample never came; no case waits for it.
	const failsafe = 30 * time.Second
	tests := []struct {
		name        string
		interval    time.Duration
		setInterval time.Duration
		// cancelOn ends the run from the callback of that sample: the 4th is the second process
		// scan. 0 leaves the end to cancelAfter.
		cancelOn       int32
		cancelAfter    time.Duration
		wantMinSamples int32
		wantProcRates  bool // on the last sample: set once the second process scan has run
		detail         Detail
	}{
		{name: "samples until cancelled", interval: 20 * time.Millisecond, cancelOn: 4, wantMinSamples: 4, wantProcRates: true},
		{name: "cancelled before first tick", interval: time.Hour, cancelAfter: 10 * time.Millisecond, wantMinSamples: 0},
		{
			name: "SetInterval replaces a long interval", interval: time.Hour, setInterval: 20 * time.Millisecond,
			cancelOn: 4, wantMinSamples: 4, wantProcRates: true,
		},
		{
			name: "SetDetail switches the detail collectors on", interval: 20 * time.Millisecond, cancelOn: 4,
			wantMinSamples: 4, wantProcRates: true, detail: Detail{Temps: true, Bluetooth: true, NetInfo: true, SMART: true, Dev: true},
		},
	}
	// The SMC is closed on some machines; everywhere else an open flag must fill its field.
	_, tempsErr := native.Temps()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), cmp.Or(tt.cancelAfter, failsafe))
			defer cancel()
			var calls atomic.Int32
			var last atomic.Pointer[Snapshot]

			sampler := NewSampler(tt.interval, func(s *Snapshot) {
				last.Store(s)
				if calls.Add(1) == tt.cancelOn {
					cancel()
				}
			})
			if tt.setInterval > 0 {
				sampler.SetInterval(tt.setInterval)
			}
			if tt.detail != (Detail{}) {
				sampler.SetDetail(tt.detail)
			}

			sampler.Run(ctx)

			assert.GreaterOrEqual(t, calls.Load(), tt.wantMinSamples)
			assert.Equal(t, tt.wantProcRates, last.Load() != nil && last.Load().HasProcRates)
			if last.Load() == nil {
				return
			}
			snap := last.Load()
			// Bluetooth is read behind a closed panel too, from the second sample on.
			assert.Equal(t, tt.detail.Bluetooth || calls.Load() > 1, !sampler.bluetoothRead.IsZero())
			if !tt.detail.Dev {
				assert.Empty(t, snap.Agents)
				assert.Nil(t, sampler.agents)
			}
			assert.Equal(t, tt.detail.NetInfo, snap.NetInfo != nil)
			assert.Equal(t, tt.detail.SMART, snap.Disk.Model != "")
			assert.Equal(t, tt.detail.Temps && tempsErr == nil, len(snap.Temps) > 0)
			assert.NotEmpty(t, snap.Disk.Volumes)
			// Nothing in a test run starts a recording or switches the power mode.
			mic, micPIDs, _, camera := native.MediaUse()
			assert.Equal(t, mic, snap.Media.Mic)
			assert.Equal(t, camera, snap.Media.Camera)
			assert.Len(t, snap.Media.MicBundles, len(snap.Media.MicPIDs))
			assert.Len(t, snap.Media.MicPIDs, len(micPIDs))
			assert.Equal(t, native.LowPowerMode(), snap.Battery.LowPower)
			assert.NotEmpty(t, snap.CPU.Model)
			cores := 0
			for _, c := range snap.CPU.Clusters {
				cores += c.Cores
				// An idle cluster has no live key and reads 0 even with the flag on.
				if !tt.detail.Temps {
					assert.Zero(t, c.Temp, "cluster %s", c.Name)
				}
			}
			if len(snap.CPU.Clusters) > 0 {
				assert.Len(t, snap.CPU.PerCore, cores)
			}
		})
	}
}
