package nettools

import (
	"net"
	"regexp"
	"strings"
)

var reMAC = regexp.MustCompile(`\b[0-9A-Fa-f]{1,2}(?:[:-][0-9A-Fa-f]{1,2}){5}\b`)

// parseNeighborTable extracts IPv4 -> MAC pairs from /proc/net/arp,
// `ip neigh`, `arp -a` (Windows) or `arp -an` (macOS) output. Incomplete,
// broadcast and multicast entries are skipped. MACs are returned in
// lower-case colon notation.
func parseNeighborTable(out string) map[string]string {
	res := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		ipS := reIPv4.FindString(line)
		macS := reMAC.FindString(line)
		if ipS == "" || macS == "" {
			continue
		}
		ip := net.ParseIP(ipS).To4()
		if ip == nil || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
			continue
		}
		mac := canonicalMAC(macS)
		if mac == "" || mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" || strings.HasPrefix(mac, "01:00:5e") {
			continue
		}
		res[ip.String()] = mac
	}
	return res
}

// canonicalMAC normalises "a:b:c:d:e:f" or "AA-BB-CC-DD-EE-FF" to
// "0a:0b:0c:0d:0e:0f" / "aa:bb:cc:dd:ee:ff".
func canonicalMAC(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == '-' })
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			p = "0" + p
		}
		parts[i] = strings.ToLower(p)
	}
	return strings.Join(parts, ":")
}
