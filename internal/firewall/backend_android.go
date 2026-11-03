//go:build android

package firewall

// DefaultBackend returns the firewall backend for this platform. On Android
// the rules are enforced by MiddleMan's VPN, which asks Firewall.Blocks
// about each connection; the backend only records them.
func DefaultBackend() Backend {
	return &memoryBackend{caps: Capabilities{
		PerApp: true, PerDestination: true, Backend: "MiddleMan VPN",
		Notes: "Enforced by MiddleMan's on-device VPN",
	}}
}
