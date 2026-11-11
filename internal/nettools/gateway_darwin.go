//go:build darwin && !ios

package nettools

import (
	"context"
	"os"
	"time"
)

// systemGatewayDNS returns the default gateway and DNS servers on macOS.
func systemGatewayDNS(string) (string, []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var gw string
	if out, err := runCmd(ctx, "route", "-n", "get", "default"); err == nil {
		gw = parseKeyValueIP(string(out), "gateway")
	}
	var dns []string
	if out, err := runCmd(ctx, "scutil", "--dns"); err == nil {
		dns = parseScutilDNS(string(out))
	}
	if len(dns) == 0 {
		if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
			dns = parseResolvConf(string(b))
		}
	}
	return gw, dns
}
