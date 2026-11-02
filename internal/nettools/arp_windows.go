//go:build windows

package nettools

import (
	"context"
	"time"
)

// readNeighbors returns the ARP table via `arp -a`.
func readNeighbors(ctx context.Context) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := runCmd(ctx, "arp", "-a")
	if err != nil && len(out) == 0 {
		return map[string]string{}
	}
	return parseNeighborTable(string(out))
}
