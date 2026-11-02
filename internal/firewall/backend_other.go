//go:build !linux && !windows && !(darwin && !ios) && !android

package firewall

// unsupportedBackend is used where MiddleLAN can't enforce rules.
type unsupportedBackend struct{}

// DefaultBackend returns the firewall backend for this platform.
func DefaultBackend() Backend { return unsupportedBackend{} }

func (unsupportedBackend) Name() string { return "unsupported" }

func (unsupportedBackend) Capabilities() Capabilities {
	return Capabilities{Backend: "unsupported", Notes: "The firewall isn't available on this platform."}
}

func (unsupportedBackend) Apply([]Rule) error { return ErrUnsupported }

// Clear succeeds: nothing can have been created on this platform.
func (unsupportedBackend) Clear() error { return nil }

func (unsupportedBackend) SystemFirewallOff() bool { return false }
