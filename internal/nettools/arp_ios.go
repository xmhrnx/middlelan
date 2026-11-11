//go:build ios

package nettools

import "context"

// readNeighbors returns nothing on iOS: apps can't run arp, and the
// system hides other devices' MAC addresses from them.
func readNeighbors(context.Context) map[string]string { return map[string]string{} }
