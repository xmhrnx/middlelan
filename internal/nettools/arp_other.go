//go:build !linux && !darwin && !windows

package nettools

import "context"

// readNeighbors is not implemented on this platform.
func readNeighbors(context.Context) map[string]string { return map[string]string{} }
