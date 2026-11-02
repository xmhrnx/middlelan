//go:build !windows

package firewall

import (
	"net/netip"
	"os"
)

// resolvConfPaths lists the files naming DNS servers: the stub resolver's
// file and systemd-resolved's list of upstream servers.
var resolvConfPaths = []string{"/etc/resolv.conf", "/run/systemd/resolve/resolv.conf"}

// systemDNSServers returns the configured DNS servers. Unreadable files are
// skipped: a missing list only means fewer addresses are protected beyond
// loopback and DHCP, which are always protected.
func systemDNSServers() []netip.Addr {
	var out []netip.Addr
	for _, p := range resolvConfPaths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out = append(out, parseResolvConf(string(b))...)
	}
	return dedupeAddrs(out)
}
