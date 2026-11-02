//go:build !windows

package nettools

import (
	"net"
	"time"
)

// platformPingers returns no extra techniques outside Windows; ICMP sockets
// and the system ping binary cover these platforms.
func platformPingers() []pingerFactory { return nil }

// nativeEcho is only available on Windows; elsewhere the LAN scanner uses
// an ICMP socket sweep instead.
func nativeEcho(net.IP, time.Duration) (bool, time.Duration, error) {
	return false, 0, errUnsupported
}
