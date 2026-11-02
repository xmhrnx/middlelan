//go:build linux

package nettools

import (
	"context"
	"os"
	"time"
)

// readNeighbors returns the kernel ARP table. On Android 10+ both sources
// are usually denied to apps; an empty map is returned then.
func readNeighbors(ctx context.Context) map[string]string {
	if b, err := os.ReadFile("/proc/net/arp"); err == nil {
		if m := parseNeighborTable(string(b)); len(m) > 0 {
			return m
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if out, err := runCmd(ctx, "ip", "-4", "neigh", "show"); err == nil {
		return parseNeighborTable(string(out))
	}
	return map[string]string{}
}
