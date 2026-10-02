package netinspect

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	traceURL      = "https://1.1.1.1/cdn-cgi/trace"
	traceTimeout  = 5 * time.Second
	traceMaxBytes = 4 << 10
)

// PublicIP asks Cloudflare which address this Mac is seen from, and only when the user
// clicks for it: one GET, no redirects, no proxy, no connection kept open afterwards.
func PublicIP(ctx context.Context) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, traceTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, traceURL, http.NoBody)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("build public ip request: %w", err)
	}
	client := &http.Client{
		Transport:     &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("request public ip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf("request public ip: status %d", resp.StatusCode)
	}
	return parseTrace(io.LimitReader(resp.Body, traceMaxBytes))
}

// parseTrace finds the ip= line of a Cloudflare trace; the answer is remote input, so
// nothing but a parsed address leaves this function.
func parseTrace(r io.Reader) (netip.Addr, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		value, ok := strings.CutPrefix(sc.Text(), "ip=")
		if !ok {
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("parse trace address: %w", err)
		}
		return addr, nil
	}
	if err := sc.Err(); err != nil {
		return netip.Addr{}, fmt.Errorf("read trace: %w", err)
	}
	return netip.Addr{}, errors.New("trace has no ip line")
}
