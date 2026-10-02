package netinspect

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	lookupTimeout  = time.Second
	lookupInFlight = 4
)

// resolver caches PTR names for the process lifetime; "" is a negative entry.
// No TTL; add one if a stale PTR ever shows up.
type resolver struct {
	mu     sync.Mutex
	cache  map[string]string
	sem    chan struct{}
	lookup func(ctx context.Context, ip string) ([]string, error)
}

func newResolver() *resolver {
	return &resolver{
		cache:  map[string]string{},
		sem:    make(chan struct{}, lookupInFlight),
		lookup: net.DefaultResolver.LookupAddr,
	}
}

// Lookup returns the PTR name or "" for private, unresolvable and unparsable
// addresses; callers fall back to the IP.
func (r *resolver) Lookup(ctx context.Context, ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() ||
		parsed.IsUnspecified() || parsed.IsMulticast() {
		return ""
	}
	r.mu.Lock()
	host, ok := r.cache[ip]
	r.mu.Unlock()
	if ok {
		return host
	}
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return ""
	}
	defer func() { <-r.sem }()
	lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	names, err := r.lookup(lctx, ip)
	if ctx.Err() != nil {
		return ""
	}
	if err == nil && len(names) > 0 {
		host = strings.TrimSuffix(names[0], ".")
	}
	r.mu.Lock()
	r.cache[ip] = host
	r.mu.Unlock()
	return host
}
