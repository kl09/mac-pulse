package netinspect

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // one table for the function under test; the length is the cases.
func TestParseNettop(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/nettop.txt")
	require.NoError(t, err)

	tests := []struct {
		name          string
		input         string
		wantProcs     int
		wantConns     int
		wantListening int
		wantProc      map[int32]byteCount
		wantInConns   []Conn
		wantInListen  []Listener
	}{
		{
			name:          "captured fixture",
			input:         string(fixture),
			wantProcs:     37,
			wantConns:     85,
			wantListening: 30,
			wantProc:      map[int32]byteCount{353: {In: 47906, Out: 578221}, 91900: {In: 1, Out: 2}, 90985: {In: 3, Out: 4}},
			wantInConns: []Conn{
				{
					PID: 353, Proto: "tcp4", Local: "192.0.2.5:62832", RemoteIP: "198.51.100.133", RemotePort: "5223",
					State: "Established", BytesIn: 47906, BytesOut: 578221,
				},
				{PID: 509, Proto: "udp6", Local: "*:5353", RemoteIP: "*", RemotePort: "*", BytesIn: 21839690, BytesOut: 12703517},
				{
					PID: 616, Proto: "tcp6", Local: "fe80::c1%utun4:1046", RemoteIP: "fe80::c2%utun4",
					RemotePort: "1025", State: "Established", BytesIn: 85251, BytesOut: 18652,
				},
				{PID: 509, Proto: "quic4", Local: "192.0.2.5:49906", RemoteIP: "198.51.100.4", RemotePort: "443", BytesIn: 2940, BytesOut: 4687},
				{
					PID: 91900, Proto: "tcp4", Local: "192.0.2.5:50001", RemoteIP: "203.0.113.4", RemotePort: "443",
					State: "Established", BytesIn: 1, BytesOut: 2,
				},
				{
					PID: 90985, Proto: "tcp4", Local: "192.0.2.5:50002", RemoteIP: "203.0.113.5", RemotePort: "443",
					State: "Established", BytesIn: 3, BytesOut: 4,
				},
			},
			wantInListen: []Listener{
				{PID: 577, Proto: "tcp", Addr: "*", Port: "5000"},
				{PID: 558, Proto: "tcp", Addr: "127.0.0.1", Port: "49153"},
				{PID: 509, Proto: "udp", Addr: "*", Port: "5353"},
			},
		},
		{name: "header only", input: ",state,bytes_in,bytes_out,\n"},
		{name: "empty input", input: ""},
		{
			name:      "a name with <-> is a process, not a socket",
			input:     ",state,bytes_in,bytes_out,\na<->b.123,,5,6,\ntcp4 1.1.1.1:1<->2.2.2.2:2,Established,5,6,\n",
			wantProcs: 1,
			wantConns: 1,
			wantProc:  map[int32]byteCount{123: {In: 5, Out: 6}},
			wantInConns: []Conn{
				{PID: 123, Proto: "tcp4", Local: "1.1.1.1:1", RemoteIP: "2.2.2.2", RemotePort: "2", State: "Established", BytesIn: 5, BytesOut: 6},
			},
		},
		{
			name:      "negative and oversized pids are not processes",
			input:     ",state,bytes_in,bytes_out,\nx.-5,,1,2,\ntcp4 1.1.1.1:1<->2.2.2.2:2,Established,1,2,\ny.4294967296,,1,2,\nz.0,,1,2,\n",
			wantConns: 1,
			wantInConns: []Conn{
				{PID: 0, Proto: "tcp4", Local: "1.1.1.1:1", RemoteIP: "2.2.2.2", RemotePort: "2", State: "Established", BytesIn: 1, BytesOut: 2},
			},
		},
		{
			name: "a newline in a process name ends the process before it: the forger's socket is nobody's, not its neighbour's",
			input: ",state,bytes_in,bytes_out,\nneighbour.100,,1,2,\na\ntcp<->.200,,0,0,\n" +
				"tcp4 127.0.0.1:47812<->*:*,Listen,,,\ntcp4 10.0.0.1:5<->2.2.2.2:443,Established,7,8,\n",
			wantProcs:     1,
			wantConns:     1,
			wantListening: 1,
			wantProc:      map[int32]byteCount{100: {In: 1, Out: 2}},
			wantInConns: []Conn{
				{PID: 0, Proto: "tcp4", Local: "10.0.0.1:5", RemoteIP: "2.2.2.2", RemotePort: "443", State: "Established", BytesIn: 7, BytesOut: 8},
			},
			wantInListen: []Listener{{PID: 0, Proto: "tcp", Addr: "127.0.0.1", Port: "47812"}},
		},
		{
			name:      "a line too short to be a row ends the process: the socket after it is nobody's",
			input:     ",state,bytes_in,bytes_out,\nneighbour.100,,1,2,\nbroken\ntcp4 10.0.0.1:5<->2.2.2.2:443,Established,7,8,\n",
			wantProcs: 1, wantConns: 1,
			wantInConns: []Conn{
				{PID: 0, Proto: "tcp4", Local: "10.0.0.1:5", RemoteIP: "2.2.2.2", RemotePort: "443", State: "Established", BytesIn: 7, BytesOut: 8},
			},
		},
		{
			name:      "a socket line without two endpoints ends the process as well",
			input:     ",state,bytes_in,bytes_out,\nneighbour.100,,1,2,\ntcp4 <->2.2.2.2:443,Established,7,8,\nudp4 *:53<->*:*,,,,\n",
			wantProcs: 1, wantListening: 1,
			wantInListen: []Listener{{PID: 0, Proto: "udp", Addr: "*", Port: "53"}},
		},
		{
			name:      "a line longer than 64 KB does not stop the scan",
			input:     ",state,bytes_in,bytes_out,\n" + strings.Repeat("a", 100000) + ".7,,1,2,\nb.8,,1,2,\n",
			wantProcs: 2,
			wantProc:  map[int32]byteCount{7: {In: 1, Out: 2}, 8: {In: 1, Out: 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			procs, conns, listening, err := parseNettop(strings.NewReader(tt.input))

			require.NoError(t, err)
			assert.Len(t, procs, tt.wantProcs)
			assert.Len(t, conns, tt.wantConns)
			assert.Len(t, listening, tt.wantListening)
			for pid, want := range tt.wantProc {
				assert.Equal(t, want, procs[pid])
			}
			for _, want := range tt.wantInConns {
				assert.Contains(t, conns, want)
			}
			for _, want := range tt.wantInListen {
				assert.Contains(t, listening, want)
			}
			for _, c := range conns {
				assert.NotEqual(t, "Listen", c.State)
			}
		})
	}
}

func TestRates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		prev map[string]byteCount
		cur  map[string]byteCount
		dt   time.Duration
		want map[string]byteRate
	}{
		{
			name: "growth",
			prev: map[string]byteCount{"a": {In: 100, Out: 10}},
			cur:  map[string]byteCount{"a": {In: 300, Out: 10}},
			dt:   2 * time.Second,
			want: map[string]byteRate{"a": {In: 100, Out: 0}},
		},
		{
			name: "negative delta reads zero",
			prev: map[string]byteCount{"a": {In: 500, Out: 500}},
			cur:  map[string]byteCount{"a": {In: 100, Out: 600}},
			dt:   time.Second,
			want: map[string]byteRate{"a": {In: 0, Out: 100}},
		},
		{
			name: "new key reads zero",
			prev: map[string]byteCount{},
			cur:  map[string]byteCount{"b": {In: 100, Out: 100}},
			dt:   time.Second,
			want: map[string]byteRate{"b": {}},
		},
		{
			name: "zero dt reads zero",
			prev: map[string]byteCount{"a": {In: 1}},
			cur:  map[string]byteCount{"a": {In: 5}},
			dt:   0,
			want: map[string]byteRate{"a": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, rates(tt.prev, tt.cur, tt.dt))
		})
	}
}
