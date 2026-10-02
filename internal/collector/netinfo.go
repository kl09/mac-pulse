package collector

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"slices"

	"github.com/kl09/mac-pulse/internal/native"
)

var (
	routeGateway   = regexp.MustCompile(`(?m)^\s*gateway: (\S+)$`)
	routeInterface = regexp.MustCompile(`(?m)^\s*interface: (\S+)$`)
	dnsServer      = regexp.MustCompile(`(?m)^\s*nameserver\[\d+\] : (\S+)$`)
)

// readNetInfo costs ~25 ms (two process launches and CoreWLAN). A source that fails leaves
// its part empty: no default route and no resolver are ordinary states of a laptop.
func readNetInfo(ctx context.Context) (*NetInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	info := &NetInfo{}
	var primary string
	var errs []error
	// route exits 1 with "not in table" when there is no default route.
	if out, err := exec.CommandContext(ctx, "route", "-n", "get", "default").Output(); err == nil {
		info.Router, primary = parseRoute(out)
	}
	if out, err := exec.CommandContext(ctx, "scutil", "--dns").Output(); err != nil {
		errs = append(errs, fmt.Errorf("scutil dns: %w", err))
	} else {
		info.DNS = parseDNS(out)
	}
	if wifi, err := native.WiFiInfo(); err == nil {
		info.WiFi = &wifi
	}
	nics, err := net.Interfaces()
	if err != nil {
		return info, errors.Join(append(errs, fmt.Errorf("list interfaces: %w", err))...)
	}
	for _, nic := range nics {
		addrs, err := nic.Addrs()
		if err != nil || len(addrs) == 0 || nic.Flags&net.FlagUp == 0 || virtualNIC(nic.Name) {
			continue
		}
		i := Interface{Name: nic.Name, MAC: nic.HardwareAddr.String(), Primary: nic.Name == primary}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			switch {
			case !ok:
			case ipNet.IP.To4() != nil:
				i.IPv4 = append(i.IPv4, ipNet.IP.String())
			default:
				i.IPv6 = append(i.IPv6, ipNet.IP.String())
			}
		}
		info.Interfaces = append(info.Interfaces, i)
	}
	slices.SortFunc(info.Interfaces, func(x, y Interface) int {
		if x.Primary != y.Primary {
			if x.Primary {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.Name, y.Name)
	})
	return info, errors.Join(errs...)
}

// parseRoute reads `route -n get default`. A gateway that is not an address ("link#14" on
// a point-to-point link) is no router to show.
func parseRoute(raw []byte) (router, iface string) {
	if m := routeGateway.FindSubmatch(raw); m != nil {
		if _, err := netip.ParseAddr(string(m[1])); err == nil {
			router = string(m[1])
		}
	}
	if m := routeInterface.FindSubmatch(raw); m != nil {
		iface = string(m[1])
	}
	return router, iface
}

// parseDNS reads `scutil --dns`, which repeats every resolver in its scoped section.
func parseDNS(raw []byte) []string {
	var servers []string
	for _, m := range dnsServer.FindAllSubmatch(raw, -1) {
		server := string(m[1])
		if _, err := netip.ParseAddr(server); err == nil && !slices.Contains(servers, server) {
			servers = append(servers, server)
		}
	}
	return servers
}
