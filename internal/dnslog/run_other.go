//go:build !linux && !windows && !(darwin && !ios) && !android

package dnslog

import "context"

// run does nothing: this platform has no supported capture method.
func (l *Log) run(ctx context.Context) error {
	l.setMethod("Unavailable on this platform", false)
	return nil
}
