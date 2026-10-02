package netinspect

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePing(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/ping.txt")
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr error
	}{
		{name: "full fixture", input: string(fixture), want: 14646 * time.Microsecond},
		{
			name: "timeout prints statistics only",
			input: "PING 1.1.1.1 (1.1.1.1): 56 data bytes\n\n--- 1.1.1.1 ping statistics ---\n" +
				"1 packets transmitted, 0 packets received, 100.0% packet loss\n",
			wantErr: ErrNoReply,
		},
		{name: "empty input", input: "", wantErr: ErrNoReply},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePing([]byte(tt.input))

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}
