package collector

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/net"
)

func readNetwork(ctx context.Context) (Network, error) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return Network{}, fmt.Errorf("net io counters: %w", err)
	}
	var n Network
	for _, c := range counters {
		if virtualNIC(c.Name) {
			continue
		}
		n.BytesRecv += c.BytesRecv
		n.BytesSent += c.BytesSent
	}
	return n, nil
}

// virtualNIC is true for loopback, tunnels, AWDL, bridges and VM NICs: they carry bytes
// another interface already counts.
func virtualNIC(name string) bool {
	return name == "lo0" || slices.ContainsFunc([]string{"utun", "awdl", "bridge", "vmenet", "llw"}, func(prefix string) bool {
		return strings.HasPrefix(name, prefix)
	})
}
