package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRoute(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/route_default.txt")
	require.NoError(t, err)

	tests := []struct {
		name       string
		input      string
		wantRouter string
		wantIface  string
	}{
		{name: "full fixture", input: string(fixture), wantRouter: "192.0.2.1", wantIface: "en0"},
		{
			name:       "IPv6 gateway with a zone",
			input:      "   route to: default\n    gateway: fe80::1%en0\n  interface: en0\n",
			wantRouter: "fe80::1%en0", wantIface: "en0",
		},
		{name: "point-to-point link has an interface and no router", input: "    gateway: link#24\n  interface: utun4\n", wantIface: "utun4"},
		{name: "no default route", input: "route: writing to routing socket: not in table\n"},
		{name: "empty input", input: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			router, iface := parseRoute([]byte(tt.input))

			assert.Equal(t, tt.wantRouter, router)
			assert.Equal(t, tt.wantIface, iface)
		})
	}
}

func TestParseDNS(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/scutil_dns.txt")
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "full fixture: the scoped section repeats the resolvers", input: string(fixture), want: []string{"198.51.100.53", "203.0.113.53"}},
		{
			name:  "IPv6 resolver, and a line that is not an address is dropped",
			input: "  nameserver[0] : 2001:4860:4860::8888\n  nameserver[1] : <script>\n  nameserver[2] : 1.1.1.1\n",
			want:  []string{"2001:4860:4860::8888", "1.1.1.1"},
		},
		{name: "no resolvers", input: "DNS configuration\n\nresolver #1\n  domain   : local\n"},
		{name: "empty input", input: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, parseDNS([]byte(tt.input)))
		})
	}
}
