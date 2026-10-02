package netinspect

import (
	"math"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/collector"
)

// Live: nettop has to see a socket this test has just opened.
func TestListening(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name    string
		network string
	}{
		{name: "a TCP listener of this process, with its port and working directory", network: "tcp"},
		{name: "a bound UDP socket counts as listening", network: "udp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var addr net.Addr
			if tt.network == "tcp" {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				t.Cleanup(func() { _ = l.Close() })
				addr = l.Addr()
			} else {
				c, err := net.ListenPacket("udp", "127.0.0.1:0")
				require.NoError(t, err)
				t.Cleanup(func() { _ = c.Close() })
				addr = c.LocalAddr()
			}
			_, port, err := net.SplitHostPort(addr.String())
			require.NoError(t, err)

			got, err := Listening(t.Context())

			require.NoError(t, err)
			var own []Listener
			for _, l := range got {
				if l.PID == int32(os.Getpid()) && l.Port == port {
					assert.NotEmpty(t, l.App)
					l.App = ""
					own = append(own, l)
				}
			}
			want := Listener{PID: int32(os.Getpid()), Proto: tt.network, Addr: "127.0.0.1", Port: port, Dir: collector.TildePath(home, cwd)}
			assert.Equal(t, []Listener{want}, own)
		})
	}
}

func TestAppFor(t *testing.T) {
	t.Parallel()

	self := int32(os.Getpid())
	owner, ownerBundle := collector.AppOf(t.Context(), self)
	require.NotEmpty(t, owner)
	group := func(pid int32) (string, string, bool) {
		return "Google Chrome", "/Applications/Google Chrome.app", pid == self
	}

	tests := []struct {
		name   string
		pid    int32
		groups func(int32) (string, string, bool)
		named  map[int32]appID
		want   appID
	}{
		{
			name: "a pid the last scan knows takes its group, over the name of an earlier poll", pid: self, groups: group,
			named: map[int32]appID{self: {name: "codex"}}, want: appID{name: "Google Chrome", bundlePath: "/Applications/Google Chrome.app"},
		},
		{
			name: "a pid the scan has not seen keeps the name of an earlier poll", pid: 7, groups: group,
			named: map[int32]appID{7: {name: "curl"}}, want: appID{name: "curl"},
		},
		{name: "a pid nobody has named is read from the process table", pid: self, want: appID{name: owner, bundlePath: ownerBundle}},
		{name: "a pid that is gone has no name", pid: math.MaxInt32, groups: group},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, appFor(t.Context(), tt.pid, tt.groups, tt.named))
		})
	}
}

func TestReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   sample
		want *Report
	}{
		{
			name: "two pids of one app sum rates and totals",
			in: sample{
				procs:     map[int32]byteCount{1: {In: 100, Out: 10}, 2: {In: 200, Out: 20}},
				base:      map[int32]byteCount{1: {}, 2: {}},
				procRates: map[int32]byteRate{1: {In: 5, Out: 1}, 2: {In: 7, Out: 2}},
				names:     map[int32]string{1: "Safari", 2: "Safari"},
				bundles:   map[string]string{"Safari": "/Applications/Safari.app"},
			},
			want: &Report{Apps: []AppTraffic{{
				App: "Safari", BundlePath: "/Applications/Safari.app", PIDs: []int32{1, 2},
				DownRate: 12, UpRate: 3, DownTotal: 300, UpTotal: 30,
			}}},
		},
		{
			name: "totals subtract the first-seen baseline and never go below zero",
			in: sample{
				procs: map[int32]byteCount{1: {In: 500, Out: 50}},
				base:  map[int32]byteCount{1: {In: 100, Out: 80}},
				names: map[int32]string{1: "curl"},
			},
			want: &Report{Apps: []AppTraffic{{App: "curl", PIDs: []int32{1}, DownTotal: 400, UpTotal: 0}}},
		},
		{
			name: "talkers aggregate by remote IP and skip the wildcard",
			in: sample{
				procs: map[int32]byteCount{1: {}, 2: {}},
				names: map[int32]string{1: "a", 2: "b"},
				conns: []Conn{
					{PID: 1, Proto: "tcp4", Local: "l:1", RemoteIP: "1.1.1.1", RemotePort: "443", Host: "one.one"},
					{PID: 2, Proto: "tcp4", Local: "l:2", RemoteIP: "1.1.1.1", RemotePort: "443", Host: "one.one"},
					{PID: 1, Proto: "udp4", Local: "*:5353", RemoteIP: "*", RemotePort: "*"},
				},
				connRates: map[string]byteRate{
					"tcp4 l:1<->1.1.1.1:443": {In: 10, Out: 1},
					"tcp4 l:2<->1.1.1.1:443": {In: 20, Out: 2},
				},
			},
			want: &Report{
				Apps: []AppTraffic{{App: "a", PIDs: []int32{1}, Conns: 2}, {App: "b", PIDs: []int32{2}, Conns: 1}},
				Conns: []Conn{
					{App: "b", PID: 2, Proto: "tcp4", Local: "l:2", RemoteIP: "1.1.1.1", RemotePort: "443", Host: "one.one", DownRate: 20, UpRate: 2},
					{App: "a", PID: 1, Proto: "tcp4", Local: "l:1", RemoteIP: "1.1.1.1", RemotePort: "443", Host: "one.one", DownRate: 10, UpRate: 1},
					{App: "a", PID: 1, Proto: "udp4", Local: "*:5353", RemoteIP: "*", RemotePort: "*"},
				},
				Talkers: []Talker{{IP: "1.1.1.1", Host: "one.one", Apps: []string{"a", "b"}, DownRate: 30, UpRate: 3, Conns: 2}},
			},
		},
		{
			name: "listening gets the app name and its directory, except the root every GUI app sits in",
			in: sample{
				procs: map[int32]byteCount{7: {}, 8: {}, 9: {}},
				names: map[int32]string{7: "python3", 8: "Safari", 9: "sshd"},
				dirs:  map[int32]string{7: "~/src/site", 8: "/"},
				listening: []Listener{
					{PID: 7, Proto: "tcp", Addr: "*", Port: "8123"},
					{PID: 8, Proto: "tcp", Addr: "*", Port: "7000"},
					{PID: 9, Proto: "tcp", Addr: "*", Port: "22"},
				},
			},
			want: &Report{
				Apps: []AppTraffic{{App: "Safari", PIDs: []int32{8}}, {App: "python3", PIDs: []int32{7}}, {App: "sshd", PIDs: []int32{9}}},
				Listening: []Listener{
					{App: "Safari", PID: 8, Proto: "tcp", Addr: "*", Port: "7000"},
					{App: "python3", PID: 7, Proto: "tcp", Addr: "*", Port: "8123", Dir: "~/src/site"},
					{App: "sshd", PID: 9, Proto: "tcp", Addr: "*", Port: "22"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, report(tt.in))
		})
	}
}
