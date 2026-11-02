//go:build android

package firewall

// DefaultBackend returns the firewall backend for this platform. On Android
// the rules are enforced by MiddleLAN's VPN, which asks Firewall.Blocks
// about each connection; the backend only records them.
func DefaultBackend() Backend {
	return &memoryBackend{caps: Capabilities{
		PerApp: true, PerDestination: true, Backend: "MiddleLAN VPN",
		Notes: "Enforced by MiddleLAN's on-device VPN",
	}}
}
