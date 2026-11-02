//go:build !linux

package vpn

// stackState is the platform part of a run; the engine only runs on Linux
// (and Android).
type stackState struct{}

// Start returns ErrUnsupported: the engine needs a Linux TUN descriptor.
func (e *Engine) Start(fd int, mtu int, dns []string) error { return ErrUnsupported }

// Stop does nothing where the engine can't run.
func (e *Engine) Stop() error { return nil }
