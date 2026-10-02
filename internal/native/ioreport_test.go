package native

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPower(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	// The first call in the process only subscribes.
	_, _ = ReadPower()
	time.Sleep(time.Second)
	power, err := ReadPower()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("IOReport is closed on this machine")
	}
	require.NoError(t, err)

	assert.Greater(t, power.CPUW, 0.05)
	assert.Less(t, power.CPUW, 60.0)
	assert.GreaterOrEqual(t, power.GPUW, 0.0)
	assert.Less(t, power.GPUW, 60.0)
	for name, mhz := range map[string]float64{"efficiency": power.EMHz, "performance": power.PMHz, "gpu": power.GPUMHz} {
		if mhz != 0 {
			assert.Greater(t, mhz, 300.0, name)
			assert.Less(t, mhz, 6000.0, name)
		}
	}
}

func BenchmarkReadPower(b *testing.B) {
	_, _ = ReadPower()
	for b.Loop() {
		_, _ = ReadPower()
	}
}
