package netinspect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		conns []Conn
		want  map[flow]byteCount
	}{
		{name: "no sockets", want: map[flow]byteCount{}},
		{
			name: "sockets are keyed by pid and endpoints",
			conns: []Conn{
				{PID: 1, Proto: "tcp4", Local: "10.0.0.5:50001", RemoteIP: "1.1.1.1", RemotePort: "443", BytesIn: 100, BytesOut: 10},
				{PID: 2, Proto: "udp4", Local: "*:5353", RemoteIP: "*", RemotePort: "*", BytesIn: 7, BytesOut: 3},
				{PID: 3, Proto: "udp4", Local: "*:5353", RemoteIP: "*", RemotePort: "*", BytesIn: 1, BytesOut: 2},
			},
			want: map[flow]byteCount{
				{pid: 1, key: "tcp4 10.0.0.5:50001<->1.1.1.1:443"}: {In: 100, Out: 10},
				{pid: 2, key: "udp4 *:5353<->*:*"}:                 {In: 7, Out: 3},
				{pid: 3, key: "udp4 *:5353<->*:*"}:                 {In: 1, Out: 2},
			},
		},
		{
			name: "sockets with the same endpoints in one pid add up",
			conns: []Conn{
				{PID: 2, Proto: "udp4", Local: "*:5353", RemoteIP: "*", RemotePort: "*", BytesIn: 7, BytesOut: 3},
				{PID: 2, Proto: "udp4", Local: "*:5353", RemoteIP: "*", RemotePort: "*", BytesIn: 1, BytesOut: 2},
			},
			want: map[flow]byteCount{{pid: 2, key: "udp4 *:5353<->*:*"}: {In: 8, Out: 5}},
		},
		{
			name: "loopback sockets are left out",
			conns: []Conn{
				{PID: 1, Proto: "tcp4", Local: "127.0.0.1:50001", RemoteIP: "127.0.0.1", RemotePort: "8080", BytesIn: 100, BytesOut: 10},
				{PID: 1, Proto: "tcp6", Local: "::1:50002", RemoteIP: "::1", RemotePort: "8080", BytesIn: 100, BytesOut: 10},
				{PID: 1, Proto: "tcp4", Local: "10.0.0.5:50003", RemoteIP: "10.0.0.9", RemotePort: "22", BytesIn: 5, BytesOut: 6},
			},
			want: map[flow]byteCount{{pid: 1, key: "tcp4 10.0.0.5:50003<->10.0.0.9:22"}: {In: 5, Out: 6}},
		},
		{
			name: "a socket nobody could be named for is left out: pid 0 would be counted for the kernel",
			conns: []Conn{
				{PID: 0, Proto: "tcp4", Local: "10.0.0.5:50001", RemoteIP: "1.1.1.1", RemotePort: "443", BytesIn: 5000, BytesOut: 7000},
				{PID: 4, Proto: "tcp4", Local: "10.0.0.5:50002", RemoteIP: "1.1.1.1", RemotePort: "443", BytesIn: 5, BytesOut: 6},
			},
			want: map[flow]byteCount{{pid: 4, key: "tcp4 10.0.0.5:50002<->1.1.1.1:443"}: {In: 5, Out: 6}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, flows(tt.conns))
		})
	}
}

func TestFlowDeltas(t *testing.T) {
	t.Parallel()

	web := flow{pid: 1, key: "tcp4 l:1<->1.1.1.1:443"}
	api := flow{pid: 1, key: "tcp4 l:2<->2.2.2.2:443"}
	dns := flow{pid: 2, key: "udp4 *:5353<->*:*"}

	tests := []struct {
		name string
		prev map[flow]byteCount
		cur  map[flow]byteCount
		want map[int32]byteCount
	}{
		{
			name: "first poll is only a baseline",
			cur:  map[flow]byteCount{web: {In: 1000, Out: 100}},
			want: map[int32]byteCount{},
		},
		{
			name: "sockets of one pid add up and an idle pid is left out",
			prev: map[flow]byteCount{web: {In: 1000, Out: 100}, api: {In: 50, Out: 5}, dns: {In: 7, Out: 7}},
			cur:  map[flow]byteCount{web: {In: 1500, Out: 120}, api: {In: 60, Out: 5}, dns: {In: 7, Out: 7}},
			want: map[int32]byteCount{1: {In: 510, Out: 20}},
		},
		{
			name: "new socket counts in full",
			prev: map[flow]byteCount{},
			cur:  map[flow]byteCount{web: {In: 300, Out: 30}},
			want: map[int32]byteCount{1: {In: 300, Out: 30}},
		},
		{
			name: "closed socket does not pull the pid's total down",
			prev: map[flow]byteCount{web: {In: 9000, Out: 900}, api: {In: 50, Out: 5}},
			cur:  map[flow]byteCount{api: {In: 80, Out: 6}},
			want: map[int32]byteCount{1: {In: 30, Out: 1}},
		},
		{
			name: "counter below the previous reading is a new socket on the same 4-tuple",
			prev: map[flow]byteCount{web: {In: 9000, Out: 900}},
			cur:  map[flow]byteCount{web: {In: 40, Out: 1000}},
			want: map[int32]byteCount{1: {In: 40, Out: 1000}},
		},
		{
			name: "either direction below the previous reading is enough",
			prev: map[flow]byteCount{web: {In: 100, Out: 900}},
			cur:  map[flow]byteCount{web: {In: 500, Out: 10}},
			want: map[int32]byteCount{1: {In: 500, Out: 10}},
		},
		{
			name: "same endpoints under another pid are a different socket",
			prev: map[flow]byteCount{dns: {In: 500, Out: 500}},
			cur:  map[flow]byteCount{dns: {In: 500, Out: 500}, {pid: 3, key: dns.key}: {In: 10, Out: 20}},
			want: map[int32]byteCount{3: {In: 10, Out: 20}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, flowDeltas(tt.prev, tt.cur))
		})
	}
}
