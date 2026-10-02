package netinspect

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolver_Lookup(t *testing.T) {
	t.Parallel()

	errNoPTR := errors.New("no ptr")
	tests := []struct {
		name      string
		ip        string
		cache     map[string]string
		names     []string
		lookupErr error
		want      string
		wantCalls int32
	}{
		{name: "cache hit skips lookup", ip: "8.8.8.8", cache: map[string]string{"8.8.8.8": "dns.google"}, want: "dns.google"},
		{name: "resolves once then caches", ip: "203.0.113.211", names: []string{"host.example.net."}, want: "host.example.net", wantCalls: 1},
		{name: "negative result cached", ip: "198.51.100.218", lookupErr: errNoPTR, wantCalls: 1},
		{name: "private ip skipped", ip: "10.0.0.5", names: []string{"router.lan."}},
		{name: "loopback skipped", ip: "127.0.0.1", names: []string{"localhost."}},
		{name: "zoned link-local skipped", ip: "fe80::1%utun4", names: []string{"x."}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			r := newResolver()
			for ip, host := range tt.cache {
				r.cache[ip] = host
			}
			r.lookup = func(context.Context, string) ([]string, error) {
				calls.Add(1)
				return tt.names, tt.lookupErr
			}

			first := r.Lookup(t.Context(), tt.ip)
			second := r.Lookup(t.Context(), tt.ip)

			assert.Equal(t, tt.want, first)
			assert.Equal(t, tt.want, second)
			assert.Equal(t, tt.wantCalls, calls.Load())
		})
	}
}
