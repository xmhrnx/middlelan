//go:build !linux && !darwin && !windows

package nettools

// systemGatewayDNS is not implemented on this platform.
func systemGatewayDNS(string) (string, []string) { return "", nil }
