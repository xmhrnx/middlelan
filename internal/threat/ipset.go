package threat

import (
	"encoding/binary"
	"math/bits"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// list is one parsed blocklist. It is never modified once built.
type list struct {
	src     *source
	v4      []range4            // sorted, non-overlapping
	v6      []range6            // sorted, non-overlapping
	domains map[string]struct{} // lower case, no trailing dot
	details []string            // indexed by range det
	entries int

	loaded        bool // has data from the cache or a download
	updated       time.Time
	etag, lastMod string
	err           string
}

// range4 is an inclusive IPv4 range; det indexes list.details or is -1.
type range4 struct {
	lo, hi uint32
	det    int32
}

// range6 is an inclusive IPv6 range; det indexes list.details or is -1.
type range6 struct {
	lo, hi netip.Addr
	det    int32
}

// reserved4 is IPv4 space that must never be flagged: private, loopback,
// link-local, CGNAT, documentation, benchmarking, multicast and reserved.
var reserved4 = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
	"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/3",
)

// global6 is the IPv6 global unicast space; entries outside it (loopback,
// ULA, link-local, multicast, mapped ...) are skipped, as are those that
// overlap reserved6.
var (
	global6   = netip.MustParsePrefix("2000::/3")
	reserved6 = mustPrefixes("2001::/32", "2001:db8::/32", "2002::/16")
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// isReserved reports whether p overlaps space that must never be flagged.
func isReserved(p netip.Prefix) bool {
	if p.Addr().Is4() {
		for _, r := range reserved4 {
			if r.Overlaps(p) {
				return true
			}
		}
		return false
	}
	if p.Bits() < global6.Bits() || !global6.Contains(p.Addr()) {
		return true
	}
	for _, r := range reserved6 {
		if r.Overlaps(p) {
			return true
		}
	}
	return false
}

func u32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func addr4(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

// lastAddr returns the highest address in p.
func lastAddr(p netip.Prefix) netip.Addr {
	if p.Addr().Is4() {
		lo := u32(p.Addr())
		return addr4(lo | (uint32(1)<<(32-p.Bits()) - 1))
	}
	b := p.Addr().As16()
	for i := p.Bits(); i < 128; i++ {
		b[i/8] |= 0x80 >> (i % 8)
	}
	return netip.AddrFrom16(b)
}

// merge4 sorts rs and joins overlapping ranges, and adjacent ones with the
// same detail. Where ranges with different details overlap, the first
// range's detail is kept.
func merge4(rs []range4) []range4 {
	slices.SortFunc(rs, func(a, b range4) int {
		if a.lo != b.lo {
			return cmpU32(a.lo, b.lo)
		}
		return cmpU32(b.hi, a.hi)
	})
	out := rs[:0]
	for _, r := range rs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			overlap := r.lo <= last.hi
			adjacent := last.hi != ^uint32(0) && r.lo == last.hi+1 && r.det == last.det
			if overlap || adjacent {
				last.hi = max(last.hi, r.hi)
				continue
			}
		}
		out = append(out, r)
	}
	return slices.Clip(out)
}

func cmpU32(a, b uint32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// merge6 is merge4 for IPv6.
func merge6(rs []range6) []range6 {
	slices.SortFunc(rs, func(a, b range6) int {
		if c := a.lo.Compare(b.lo); c != 0 {
			return c
		}
		return b.hi.Compare(a.hi)
	})
	out := rs[:0]
	for _, r := range rs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			overlap := r.lo.Compare(last.hi) <= 0
			next := last.hi.Next()
			adjacent := next.IsValid() && next == r.lo && r.det == last.det
			if overlap || adjacent {
				if r.hi.Compare(last.hi) > 0 {
					last.hi = r.hi
				}
				continue
			}
		}
		out = append(out, r)
	}
	return slices.Clip(out)
}

// lookupIP finds a (already unmapped, zoneless) address and returns the
// match detail.
func (l *list) lookupIP(a netip.Addr) (string, bool) {
	if a.Is4() {
		v := u32(a)
		i, _ := slices.BinarySearchFunc(l.v4, v, func(r range4, v uint32) int { return cmpU32(r.lo, v) })
		// i is the first range with lo >= v; the candidate is the one
		// starting at v, or the one before.
		if i < len(l.v4) && l.v4[i].lo == v {
			return l.detail4(l.v4[i]), true
		}
		if i > 0 && l.v4[i-1].hi >= v {
			return l.detail4(l.v4[i-1]), true
		}
		return "", false
	}
	if !a.Is6() {
		return "", false
	}
	i, _ := slices.BinarySearchFunc(l.v6, a, func(r range6, a netip.Addr) int { return r.lo.Compare(a) })
	if i < len(l.v6) && l.v6[i].lo == a {
		return l.detail6(l.v6[i]), true
	}
	if i > 0 && l.v6[i-1].hi.Compare(a) >= 0 {
		return l.detail6(l.v6[i-1]), true
	}
	return "", false
}

func (l *list) detail4(r range4) string {
	if r.det >= 0 {
		return l.details[r.det]
	}
	if r.lo == r.hi {
		return ""
	}
	lo, hi := addr4(r.lo), addr4(r.hi)
	if size := uint64(r.hi-r.lo) + 1; size&(size-1) == 0 && uint64(r.lo)%size == 0 {
		return netip.PrefixFrom(lo, 32-bits.TrailingZeros64(size)).String()
	}
	return lo.String() + "-" + hi.String()
}

func (l *list) detail6(r range6) string {
	if r.det >= 0 {
		return l.details[r.det]
	}
	if r.lo == r.hi {
		return ""
	}
	for b := 0; b <= 128; b++ {
		p := netip.PrefixFrom(r.lo, b)
		if p.Masked().Addr() == r.lo && lastAddr(p) == r.hi {
			return p.String()
		}
	}
	return r.lo.String() + "-" + r.hi.String()
}

// lookupDomain checks n and its parent domains, never a bare top-level
// label, and returns the listed name.
func (l *list) lookupDomain(n string) (string, bool) {
	if len(l.domains) == 0 {
		return "", false
	}
	for strings.Contains(n, ".") {
		if _, ok := l.domains[n]; ok {
			return n, true
		}
		n = n[strings.IndexByte(n, '.')+1:]
	}
	return "", false
}
