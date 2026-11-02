//go:build linux

package nettools

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"
)

// systemGatewayDNS returns the default gateway and DNS servers on Linux and
// Android (best effort; Android may deny some sources).
func systemGatewayDNS(string) (string, []string) {
	var gw string
	if b, err := os.ReadFile("/proc/net/route"); err == nil {
		gw = parseProcNetRoute(string(b))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if gw == "" {
		if out, err := runCmd(ctx, "ip", "route", "show", "default"); err == nil {
			gw = parseIPRoute(string(out))
		}
	}
	if gw == "" {
		// Android keeps the default route in per-network policy tables.
		if out, err := runCmd(ctx, "ip", "route", "get", "8.8.8.8"); err == nil {
			gw = parseIPRoute(string(out))
		}
	}
	var dns []string
	if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		dns = parseResolvConf(string(b))
	}
	// systemd-resolved stub: prefer the real upstream list when present.
	if len(dns) == 1 && dns[0] == "127.0.0.53" {
		if b, err := os.ReadFile("/run/systemd/resolve/resolv.conf"); err == nil {
			if up := parseResolvConf(string(b)); len(up) > 0 {
				dns = up
			}
		}
	}
	if len(dns) == 0 && runtime.GOOS == "android" {
		for _, p := range []string{"net.dns1", "net.dns2"} {
			out, err := runCmd(ctx, "getprop", p)
			if err != nil {
				continue
			}
			if v := strings.TrimSpace(string(out)); v != "" {
				dns = appendUnique(dns, v)
			}
		}
	}
	return gw, dns
}
