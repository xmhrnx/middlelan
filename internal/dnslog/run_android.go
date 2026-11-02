//go:build android

package dnslog

import "context"

// run does nothing on Android: apps cannot capture packets or read the
// resolver's cache there, so the VPN parses DNS itself and calls Add.
func (l *Log) run(ctx context.Context) error {
	l.setMethod("Unavailable: turn on the VPN to see domain names", false)
	return nil
}
