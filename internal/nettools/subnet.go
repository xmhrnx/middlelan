package nettools

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// SubnetInfo is the result of a subnet calculation.
type SubnetInfo struct {
	// CIDR is the normalised network in CIDR notation.
	CIDR string
	// Network and Broadcast are the first and last addresses of the block
	// (Broadcast is empty for IPv6, which has no broadcast).
	Network, Broadcast string
	// Netmask and Wildcard are the dotted masks (IPv4) or expanded masks
	// (IPv6).
	Netmask, Wildcard string
	// FirstHost and LastHost delimit the usable host range.
	FirstHost, LastHost string
	// Class is the historical IPv4 class (A-E), or "" for IPv6.
	Class string
	// Hosts is the number of usable hosts, saturated at the uint64 maximum.
	Hosts uint64
	// Prefix is the prefix length.
	Prefix int
	// IPv6 reports an IPv6 network; Private reports RFC 1918, ULA,
	// loopback, CGNAT or link-local space.
	IPv6, Private bool
}

// CalcSubnet parses "192.168.1.10/24", "10.0.0.0 255.0.0.0", a bare IP
// address (treated as a host route) or an IPv6 CIDR and computes its
// subnet properties.
func CalcSubnet(input string) (SubnetInfo, error) {
	var info SubnetInfo
	in := strings.TrimSpace(input)
	if in == "" {
		return info, errors.New("subnet: empty input")
	}
	var addr netip.Addr
	var bits int
	fields := strings.Fields(in)
	switch {
	case len(fields) == 2:
		a, err := netip.ParseAddr(fields[0])
		if err != nil {
			return info, fmt.Errorf("subnet: %w", err)
		}
		bits, err = maskBits(fields[1], a.Is4())
		if err != nil {
			return info, err
		}
		addr = a
	case len(fields) == 1 && strings.Contains(in, "/"):
		ip, m, _ := strings.Cut(in, "/")
		a, err := netip.ParseAddr(ip)
		if err != nil {
			return info, fmt.Errorf("subnet: %w", err)
		}
		bits, err = maskBits(m, a.Is4())
		if err != nil {
			return info, err
		}
		addr = a
	case len(fields) == 1:
		a, err := netip.ParseAddr(in)
		if err != nil {
			return info, fmt.Errorf("subnet: %w", err)
		}
		addr, bits = a, a.BitLen()
	default:
		return info, fmt.Errorf("subnet: cannot parse %q", input)
	}
	addr = addr.Unmap().WithZone("")
	pfx, err := addr.Prefix(bits)
	if err != nil {
		return info, fmt.Errorf("subnet: %w", err)
	}
	total := addr.BitLen()
	info.Prefix = bits
	info.IPv6 = addr.Is6()
	info.CIDR = pfx.String()
	network := pfx.Addr()
	info.Network = network.String()

	mask := net.CIDRMask(bits, total)
	wild := make(net.IPMask, len(mask))
	for i := range mask {
		wild[i] = ^mask[i]
	}
	last := lastAddr(network, wild)
	info.Private = isPrivateAddr(addr)

	if !info.IPv6 {
		info.Netmask = net.IP(mask).String()
		info.Wildcard = net.IP(wild).String()
		info.Broadcast = last.String()
		info.Class = ipv4Class(addr)
		hostBits := total - bits
		switch {
		case hostBits == 0:
			info.Hosts = 1
			info.FirstHost, info.LastHost = network.String(), network.String()
		case hostBits == 1:
			info.Hosts = 2
			info.FirstHost, info.LastHost = network.String(), last.String()
		default:
			info.Hosts = (uint64(1) << hostBits) - 2
			info.FirstHost, info.LastHost = network.Next().String(), last.Prev().String()
		}
		return info, nil
	}

	info.Netmask = expandMask(mask)
	info.Wildcard = expandMask(wild)
	hostBits := total - bits
	if hostBits >= 64 {
		info.Hosts = math.MaxUint64
	} else {
		info.Hosts = uint64(1) << hostBits
	}
	info.FirstHost, info.LastHost = network.String(), last.String()
	if hostBits > 1 {
		// The subnet-router anycast address (all zeros) is not a host.
		info.FirstHost = network.Next().String()
		if hostBits < 64 {
			info.Hosts--
		}
	}
	return info, nil
}

// maskBits parses a prefix length or dotted IPv4 netmask.
func maskBits(m string, v4 bool) (int, error) {
	m = strings.TrimSpace(strings.TrimPrefix(m, "/"))
	maxBits := 128
	if v4 {
		maxBits = 32
	}
	if n, err := strconv.Atoi(m); err == nil {
		if n < 0 || n > maxBits {
			return 0, fmt.Errorf("subnet: prefix /%d out of range", n)
		}
		return n, nil
	}
	if !v4 {
		return 0, fmt.Errorf("subnet: invalid IPv6 prefix %q", m)
	}
	ip := net.ParseIP(m).To4()
	if ip == nil {
		return 0, fmt.Errorf("subnet: invalid netmask %q", m)
	}
	ones, bitsLen := net.IPMask(ip).Size()
	if bitsLen == 0 {
		return 0, fmt.Errorf("subnet: non-contiguous netmask %q", m)
	}
	return ones, nil
}

// lastAddr ORs network with the wildcard mask.
func lastAddr(network netip.Addr, wild net.IPMask) netip.Addr {
	b := network.AsSlice()
	for i := range b {
		b[i] |= wild[i]
	}
	a, _ := netip.AddrFromSlice(b) // same length as network: cannot fail
	return a
}

// expandMask renders an IPv6 mask as a full address.
func expandMask(m net.IPMask) string {
	a, ok := netip.AddrFromSlice(m)
	if !ok {
		return new(big.Int).SetBytes(m).Text(16)
	}
	return a.StringExpanded()
}

// ipv4Class returns the historical class of an IPv4 address.
func ipv4Class(a netip.Addr) string {
	b := a.As4()[0]
	switch {
	case b < 128:
		return "A"
	case b < 192:
		return "B"
	case b < 224:
		return "C"
	case b < 240:
		return "D"
	default:
		return "E"
	}
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// isPrivateAddr reports private, ULA, CGNAT, loopback or link-local space.
func isPrivateAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || cgnat.Contains(a)
}
