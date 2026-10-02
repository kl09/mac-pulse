package netinspect

import (
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTrace(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/trace.txt")
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    netip.Addr
		wantErr bool
	}{
		{name: "full fixture", input: string(fixture), want: netip.MustParseAddr("203.0.113.7")},
		{name: "IPv6 address", input: "h=1.1.1.1\nip=2001:db8::7\n", want: netip.MustParseAddr("2001:db8::7")},
		{name: "ip line is not an address", input: "ip=<script>alert(1)</script>\n", wantErr: true},
		{name: "no ip line", input: "h=1.1.1.1\nloc=XX\n", wantErr: true},
		{name: "key that only ends in ip is not the address", input: "warp_ip=203.0.113.7\n", wantErr: true},
		{name: "empty input", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTrace(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
