package nettools

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"net"
	"regexp"
	"strings"
)

// Pure parsers for platform network configuration output. They live in a
// file without build tags so they can be unit-tested on any OS.

// parseProcNetRoute returns the default gateway from Linux /proc/net/route.
func parseProcNetRoute(data string) string {
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || f[1] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		v := binary.LittleEndian.Uint32(b)
		if v == 0 {
			continue
		}
		return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
	}
	return ""
}

var reVia = regexp.MustCompile(`\bvia\s+(\S+)`)

// parseIPRoute returns the gateway from `ip route` / `ip route get` output.
func parseIPRoute(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "default") && !strings.Contains(line, " via ") {
			continue
		}
		if m := reVia.FindStringSubmatch(line); m != nil && net.ParseIP(m[1]) != nil {
			return m[1]
		}
	}
	return ""
}

// parseKeyValueIP returns the IP after "key:" on the first matching line,
// e.g. "gateway: 192.168.1.1" from `route -n get default`.
func parseKeyValueIP(out, key string) string {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), key) {
			v = strings.TrimSpace(v)
			if net.ParseIP(v) != nil {
				return v
			}
		}
	}
	return ""
}

// parseResolvConf returns the nameservers listed in resolv.conf content.
func parseResolvConf(data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" && net.ParseIP(stripZone(f[1])) != nil {
			out = appendUnique(out, f[1])
		}
	}
	return out
}

var reScutilNS = regexp.MustCompile(`nameserver\[\d+\]\s*:\s*(\S+)`)

// parseScutilDNS returns the nameservers in `scutil --dns` output.
func parseScutilDNS(out string) []string {
	var res []string
	for _, m := range reScutilNS.FindAllStringSubmatch(out, -1) {
		if net.ParseIP(stripZone(m[1])) != nil {
			res = appendUnique(res, m[1])
		}
	}
	return res
}

// parseRoutePrint returns the gateway of the 0.0.0.0/0 route in Windows
// `route print` output (lowest metric wins).
func parseRoutePrint(out string) string {
	best, bestMetric := "", int(^uint(0)>>1)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[0] == "0.0.0.0" && f[1] == "0.0.0.0" && net.ParseIP(f[2]) != nil {
			m := 0
			for _, c := range f[len(f)-1] {
				if c < '0' || c > '9' {
					m = bestMetric
					break
				}
				m = m*10 + int(c-'0')
			}
			if m < bestMetric || best == "" {
				best, bestMetric = f[2], m
			}
		}
	}
	return best
}

func stripZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

func appendUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}
