package firewall

import (
	"bufio"
	"net/netip"
	"strings"
)

// parseResolvConf returns the nameserver addresses in a resolv.conf file.
func parseResolvConf(content string) []netip.Addr {
	var out []netip.Addr
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		host := fields[1]
		if i := strings.IndexByte(host, '%'); i >= 0 {
			host = host[:i]
		}
		if a, err := netip.ParseAddr(host); err == nil {
			out = append(out, a.Unmap())
		}
	}
	return out
}

// dedupeAddrs removes repeated addresses, keeping the first of each.
func dedupeAddrs(in []netip.Addr) []netip.Addr {
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	for _, a := range in {
		if a.IsValid() && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}
