package native

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPU(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	gpu, err := GPU()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("no accelerator reports utilization on this machine")
	}
	require.NoError(t, err)

	assert.NotEmpty(t, gpu.Model)
	assert.Positive(t, gpu.Memory)
	for name, percent := range map[string]int{"device": gpu.Util, "renderer": gpu.Renderer, "tiler": gpu.Tiler} {
		assert.GreaterOrEqual(t, percent, 0, name)
		assert.LessOrEqual(t, percent, 100, name)
	}
}
