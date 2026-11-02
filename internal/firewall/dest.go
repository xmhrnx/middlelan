package firewall

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// destKind classifies a rule's destination.
type destKind int

const (
	destAny destKind = iota
	destPrefix
	destHost
)

// parseDest classifies a user-entered destination and normalizes it. An
// address becomes a single-address prefix and a CIDR prefix is masked.
func parseDest(s string) (destKind, netip.Prefix, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return destAny, netip.Prefix{}, "", nil
	}
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return 0, netip.Prefix{}, "", fmt.Errorf("%q isn't a valid CIDR prefix", s)
		}
		if p.Addr().Zone() != "" {
			return 0, netip.Prefix{}, "", fmt.Errorf("%q: zoned addresses aren't supported", s)
		}
		p = unmapPrefix(p).Masked()
		return destPrefix, p, p.String(), nil
	}
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return 0, netip.Prefix{}, "", fmt.Errorf("%q: zoned addresses aren't supported", s)
		}
		a = a.Unmap()
		return destPrefix, netip.PrefixFrom(a, a.BitLen()), a.String(), nil
	}
	host := strings.ToLower(strings.TrimSuffix(s, "."))
	if !validHostName(host) {
		return 0, netip.Prefix{}, "", fmt.Errorf("%q isn't an IP address, CIDR prefix or host name", s)
	}
	return destHost, netip.Prefix{}, host, nil
}

// validHostName reports whether s is a syntactically valid DNS name.
func validHostName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	// An all-numeric name is a malformed address, not a host.
	return !isAllDigitsAndDots(s)
}

// isAllDigitsAndDots reports whether s looks like a numeric address.
func isAllDigitsAndDots(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return false
		}
	}
	return true
}

// unmapPrefix turns an IPv4-mapped IPv6 prefix into its IPv4 form.
func unmapPrefix(p netip.Prefix) netip.Prefix {
	a := p.Addr()
	if !a.Is4In6() {
		return p
	}
	bits := p.Bits() - 96
	if bits < 0 {
		bits = 0
	}
	return netip.PrefixFrom(a.Unmap(), bits)
}

// minimalPrefixes sorts and de-duplicates prefixes and drops any contained
// in another, so nftables and pf sets never hold overlapping intervals.
func minimalPrefixes(in []netip.Prefix) []netip.Prefix {
	ps := make([]netip.Prefix, 0, len(in))
	for _, p := range in {
		if p.IsValid() {
			ps = append(ps, unmapPrefix(p).Masked())
		}
	}
	// Shorter (wider) prefixes first, so containment is checked against them.
	slices.SortFunc(ps, func(a, b netip.Prefix) int {
		if a.Bits() != b.Bits() {
			return a.Bits() - b.Bits()
		}
		return a.Addr().Compare(b.Addr())
	})
	var out []netip.Prefix
	for _, p := range ps {
		covered := false
		for _, q := range out {
			if q.Bits() <= p.Bits() && q.Contains(p.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b netip.Prefix) int {
		if c := a.Addr().Compare(b.Addr()); c != 0 {
			return c
		}
		return a.Bits() - b.Bits()
	})
	return out
}

// prefixString formats p, leaving out the length of a single address.
func prefixString(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}

// joinPrefixes formats prefixes as the resolved form of a destination.
func joinPrefixes(ps []netip.Prefix) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = prefixString(p)
	}
	return strings.Join(parts, ",")
}

// parseResolved parses the resolved destination backends receive: "" or a
// comma-separated list of addresses and prefixes.
func parseResolved(dest string) ([]netip.Prefix, error) {
	if strings.TrimSpace(dest) == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for _, part := range strings.Split(dest, ",") {
		kind, p, _, err := parseDest(part)
		if err != nil {
			return nil, err
		}
		if kind != destPrefix {
			return nil, fmt.Errorf("destination %q isn't resolved to addresses", part)
		}
		out = append(out, p)
	}
	return minimalPrefixes(out), nil
}

// splitFamilies separates IPv4 from IPv6 prefixes.
func splitFamilies(ps []netip.Prefix) (v4, v6 []netip.Prefix) {
	for _, p := range ps {
		if p.Addr().Is4() {
			v4 = append(v4, p)
		} else {
			v6 = append(v6, p)
		}
	}
	return v4, v6
}
