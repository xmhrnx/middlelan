//go:build !android

package vpn

// Supported reports whether this platform has the on-device VPN: only
// Android does.
func Supported() bool { return false }

// Prepare asks for the user's VPN consent (Android only).
func Prepare() (granted bool, err error) { return false, ErrUnsupported }

// Granted reports whether the user already gave VPN consent (Android only).
func Granted() bool { return false }

// Start asks the system VPN service to start (Android only).
func Start() error { return ErrUnsupported }

// Stop asks the system VPN service to stop (Android only).
func Stop() error { return ErrUnsupported }

// LastError returns why the VPN last failed to start or stopped, if it did.
func LastError() error { return nil }

func platformHooks(h Hooks) Hooks { return h }
