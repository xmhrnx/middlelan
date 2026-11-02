package firewall

import (
	"fmt"
	"net/netip"
)

// protectedRange is traffic MiddleLAN never blocks.
type protectedRange struct {
	prefix netip.Prefix
	why    string
}

// alwaysProtected are loopback and the DHCP destinations.
var alwaysProtected = []protectedRange{
	{netip.MustParsePrefix("127.0.0.0/8"), "loopback"},
	{netip.MustParsePrefix("::1/128"), "loopback"},
	{netip.MustParsePrefix("255.255.255.255/32"), "DHCP (broadcast)"},
	{netip.MustParsePrefix("ff02::1:2/128"), "DHCPv6 (all DHCP agents)"},
	{netip.MustParsePrefix("ff05::1:3/128"), "DHCPv6 (all DHCP servers)"},
}

// protectedRanges returns the always-protected ranges plus the DNS servers.
func protectedRanges(dns []netip.Addr) []protectedRange {
	out := append([]protectedRange(nil), alwaysProtected...)
	for _, a := range dns {
		a = a.Unmap()
		if a.IsValid() {
			out = append(out, protectedRange{netip.PrefixFrom(a, a.BitLen()), "the DNS server " + a.String()})
		}
	}
	return out
}

// checkProtected returns an error wrapping ErrProtected when p includes
// loopback, DHCP or one of the DNS servers.
func checkProtected(p netip.Prefix, ranges []protectedRange) error {
	for _, r := range ranges {
		if p.Overlaps(r.prefix) {
			return fmt.Errorf("%w: %s includes %s, which MiddleLAN never blocks", ErrProtected, prefixString(p), r.why)
		}
	}
	return nil
}

// isProtectedAddr reports whether a is loopback, DHCP or a DNS server.
func isProtectedAddr(a netip.Addr, ranges []protectedRange) bool {
	a = a.Unmap()
	if a.IsLoopback() {
		return true
	}
	for _, r := range ranges {
		if r.prefix.Contains(a) {
			return true
		}
	}
	return false
}
