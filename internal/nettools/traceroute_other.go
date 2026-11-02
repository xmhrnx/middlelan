//go:build !windows

package nettools

import "net"

// platformHopProber returns nil outside Windows: the system ping binary is
// used for every TTL.
func platformHopProber(net.IP) hopProber { return nil }
