package native

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcUsage(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	tests := []struct {
		name    string
		pid     int32
		wantErr error
	}{
		{name: "own process", pid: int32(os.Getpid())},
		{name: "launchd belongs to root", pid: 1, wantErr: ErrUnavailable},
		{name: "no such process", pid: 1 << 30, wantErr: ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			usage, err := ProcUsage(tt.pid)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, usage)
				return
			}
			require.NoError(t, err)
			assert.Positive(t, usage.EnergyNJ)
		})
	}
}
