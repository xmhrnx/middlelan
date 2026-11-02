//go:build !windows

package nettools

import (
	"errors"
	"syscall"
)

// isConnRefused reports whether err means the peer actively refused the
// connection (i.e. the host is up).
func isConnRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET)
}
