package adblock

import (
	"bufio"
	"bytes"
	"slices"
	"strings"
)

// rules is a parsed list: names blocked exactly (hosts files), names
// blocked with their subdomains (adblock-style ||name^) and exceptions
// (@@||name^, with subdomains). Each is sorted for binary search, which
// takes far less memory on a phone than maps.
type rules struct {
	exact, suffix, except []string
}

func (r *rules) size() int {
	if r == nil {
		return 0
	}
	return len(r.exact) + len(r.suffix)
}

func has(sorted []string, s string) bool {
	_, ok := slices.BinarySearch(sorted, s)
	return ok
}

// match reports whether name (normalized) is blocked.
func (r *rules) match(name string) bool {
	if r == nil {
		return false
	}
	for s := name; s != ""; s = parent(s) {
		if has(r.except, s) {
			return false
		}
	}
	if has(r.exact, name) {
		return true
	}
	for s := name; s != ""; s = parent(s) {
		if has(r.suffix, s) {
			return true
		}
	}
	return false
}

// merge combines lists into one.
func merge(parts []*rules) *rules {
	out := &rules{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		out.exact = append(out.exact, p.exact...)
		out.suffix = append(out.suffix, p.suffix...)
		out.except = append(out.except, p.except...)
	}
	out.exact = sortUnique(out.exact)
	out.suffix = sortUnique(out.suffix)
	out.except = sortUnique(out.except)
	return out
}

func sortUnique(s []string) []string {
	slices.Sort(s)
	return slices.Clip(slices.Compact(s))
}

// parse reads a hosts file or an adblock-style DNS filter list. Lines it
// can't use (cosmetic rules, wildcards, regular expressions, modifiers
// other than $important) are skipped.
func parse(data []byte) *rules {
	r := &rules{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '!' || line[0] == '#' || line[0] == '[' {
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case strings.HasPrefix(line, "@@||"):
			if n, ok := adblockName(line[4:]); ok {
				r.except = append(r.except, n)
			}
		case strings.HasPrefix(line, "||"):
			if n, ok := adblockName(line[2:]); ok {
				r.suffix = append(r.suffix, n)
			}
		default:
			f := strings.Fields(line)
			if len(f) < 2 || (f[0] != "0.0.0.0" && f[0] != "127.0.0.1" && f[0] != "::" && f[0] != "::1") {
				continue
			}
			for _, n := range f[1:] {
				if n = normalize(n); validName(n) && !localName[n] {
					r.exact = append(r.exact, n)
				}
			}
		}
	}
	r.exact = sortUnique(r.exact)
	r.suffix = sortUnique(r.suffix)
	r.except = sortUnique(r.except)
	return r
}

// localName are names hosts files list for the machine itself.
var localName = map[string]bool{
	"localhost": true, "localhost.localdomain": true, "local": true, "broadcasthost": true,
	"ip6-localhost": true, "ip6-loopback": true, "0.0.0.0": true, "ip6-localnet": true,
	"ip6-mcastprefix": true, "ip6-allnodes": true, "ip6-allrouters": true, "ip6-allhosts": true,
}

// adblockName takes "name^" or "name^$important" and returns the name.
func adblockName(s string) (string, bool) {
	name, rest, ok := strings.Cut(s, "^")
	if !ok || (rest != "" && rest != "$important") {
		return "", false
	}
	name = normalize(name)
	return name, validName(name)
}

// validName accepts host names with at least two labels.
func validName(n string) bool {
	if len(n) < 3 || len(n) > 253 || !strings.Contains(n, ".") || n[0] == '.' || n[0] == '-' {
		return false
	}
	for i := 0; i < len(n); i++ {
		c := n[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
			return false
		}
	}
	return !strings.Contains(n, "..")
}
