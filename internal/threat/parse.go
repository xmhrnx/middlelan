package threat

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// maxBadRatio rejects a file in which more than this share of the
// non-comment lines cannot be understood: it is not the list we expect.
const maxBadRatio = 0.10

// hostsIgnored are names found in hosts files that are not entries.
var hostsIgnored = map[string]bool{
	"localhost": true, "localhost.localdomain": true, "local": true,
	"broadcasthost": true, "ip6-localhost": true, "ip6-loopback": true,
}

// parseList parses and validates a downloaded or cached list for src.
//
// One format covers all lists: one entry per line, an IPv4/IPv6 address
// or CIDR, or for domain lists a host name or hosts-file line ("127.0.0.1
// evil.com"). Lines starting with # or ; are comments, as is anything
// after #. Text after ; is kept as the entry's detail (Spamhaus "cidr ;
// SBL123"). Blank lines and CRLF endings are fine. Entries overlapping
// reserved address space are skipped.
func parseList(src *source, data []byte) (*list, error) {
	if looksLikeHTML(data) {
		return nil, errors.New("not a blocklist (got an HTML page)")
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	l := &list{src: src, loaded: true}
	var (
		v4        []range4
		v6        []range6
		detailIdx = map[string]int32{}
		lines     int
		bad       int
	)
	if src.Domains {
		l.domains = map[string]struct{}{}
	}
	intern := func(s string) int32 {
		if s == "" {
			return -1
		}
		if i, ok := detailIdx[s]; ok {
			return i
		}
		i := int32(len(l.details))
		l.details = append(l.details, s)
		detailIdx[s] = i
		return i
	}

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		lines++
		detail := ""
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line, detail = strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			bad++
			continue
		}
		tok := fields[0]
		if src.Domains && len(fields) >= 2 {
			if _, err := netip.ParseAddr(tok); err == nil {
				tok = fields[1] // hosts file: "127.0.0.1 name"
			}
		}

		switch ok, skip := addEntry(l, tok, intern(detail), &v4, &v6); {
		case skip:
		case ok:
			l.entries++
		default:
			bad++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if lines > 0 && float64(bad) > maxBadRatio*float64(lines) {
		return nil, fmt.Errorf("not a blocklist (%d of %d lines unreadable)", bad, lines)
	}
	if l.entries < src.MinEntries {
		return nil, fmt.Errorf("too few entries (%d, expected at least %d)", l.entries, src.MinEntries)
	}
	l.v4, l.v6 = merge4(v4), merge6(v6)
	return l, nil
}

// addEntry adds one token. ok means it was added; skip means it was
// understood but deliberately left out (reserved space, hosts boilerplate).
func addEntry(l *list, tok string, det int32, v4 *[]range4, v6 *[]range6) (ok, skip bool) {
	var p netip.Prefix
	if strings.Contains(tok, "/") {
		q, err := netip.ParsePrefix(tok)
		if err != nil {
			return false, false
		}
		p = q.Masked()
	} else if a, err := netip.ParseAddr(tok); err == nil {
		a = a.WithZone("").Unmap()
		p = netip.PrefixFrom(a, a.BitLen())
	} else if l.domains != nil {
		n := normalizeDomain(tok)
		if hostsIgnored[n] || !strings.Contains(n, ".") {
			// Boilerplate, or a bare top-level name that must never match.
			return false, true
		}
		if !validDomain(n) {
			return false, false
		}
		if _, dup := l.domains[n]; dup {
			return false, true
		}
		l.domains[n] = struct{}{}
		return true, false
	} else {
		return false, false
	}

	if p.Addr().Is4In6() {
		// A mapped prefix such as ::ffff:1.2.3.0/120: treat as IPv4.
		if p.Bits() < 96 {
			return false, true
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	if isReserved(p) {
		return false, true
	}
	if p.Addr().Is4() {
		lo := u32(p.Addr())
		*v4 = append(*v4, range4{lo: lo, hi: u32(lastAddr(p)), det: det})
	} else {
		*v6 = append(*v6, range6{lo: p.Addr(), hi: lastAddr(p), det: det})
	}
	return true, false
}

// normalizeDomain lower-cases a host name and drops a trailing dot.
func normalizeDomain(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

// validDomain accepts a normalized host name with at least two labels.
func validDomain(n string) bool {
	if len(n) == 0 || len(n) > 253 || !strings.Contains(n, ".") {
		return false
	}
	for label := range strings.SplitSeq(n, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}

// looksLikeHTML spots error and login pages served instead of a list.
func looksLikeHTML(data []byte) bool {
	head := bytes.TrimPrefix(data[:min(len(data), 512)], []byte("\xef\xbb\xbf"))
	head = bytes.TrimSpace(head)
	return len(head) > 0 && head[0] == '<'
}
