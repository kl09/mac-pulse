package netinspect

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSpeed(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/networkquality.json")
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    Speed
		wantErr bool
	}{
		{
			name: "captured run: bits per second become bytes per second", input: string(fixture),
			want: Speed{Down: 5320625.5, Up: 8216264.5, RPM: 70.27931213378906, RTTms: 48.593135833740234},
		},
		{
			name: "a run without responsiveness still has its throughput", input: `{"dl_throughput":800,"ul_throughput":0}`,
			want: Speed{Down: 100},
		},
		{name: "an object without throughput is not a result", input: `{"error_code":-1009}`, wantErr: true},
		{name: "not JSON", input: "networkQuality: command failed", wantErr: true},
		{name: "empty output", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSpeed(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A real run moves ~200 MB, so only the paths that never start networkQuality are tested here.
func TestSpeedTest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		deadline time.Duration
		wantErr  error
	}{
		{name: "a run out of time reports the deadline, not the kill", deadline: -time.Second, wantErr: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), tt.deadline)
			t.Cleanup(cancel)

			_, err := SpeedTest(ctx)

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
