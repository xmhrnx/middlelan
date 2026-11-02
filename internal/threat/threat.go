// Package threat flags connections to known-malicious addresses and domains.
// It keeps free public blocklists (FireHOL level1, Spamhaus DROP, abuse.ch
// Feodo Tracker and URLhaus) cached on disk and checks them offline. Lookups
// are lock-free reads of an immutable in-memory snapshot, cheap enough to
// run for every connection.
package threat

import (
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Category says what kind of threat a list tracks.
type Category string

// Threat categories.
const (
	CategoryBotnet      Category = "botnet C2"
	CategoryMalware     Category = "malware"
	CategorySpam        Category = "spam/abuse"
	CategoryCompromised Category = "compromised"
	CategoryPhishing    Category = "phishing"
)

// Match describes a hit on a blocklist.
type Match struct {
	// Indicator is the address or domain that was checked, normalized.
	Indicator string
	// List is the name of the list it was found on.
	List string
	// Category is the list's Category, as a string.
	Category string
	// Detail is extra context: the list's reference (such as a Spamhaus
	// SBL id), the listed network, or the listed parent domain.
	Detail string
}

// ListInfo describes one blocklist and the state of its local copy.
type ListInfo struct {
	Name     string
	URL      string
	Category string
	// Entries is the number of usable entries in the local copy.
	Entries int
	// Updated is when the local copy was last downloaded or confirmed
	// current by the server; zero if there is none.
	Updated time.Time
	// Err is the last problem with the list (failed download, rejected
	// content, unusable cache); empty when the last attempt succeeded.
	Err string
}

// DB holds the lists. Lookups are lock-free reads of an immutable snapshot
// swapped on update.
type DB struct {
	dir        string // cache directory ("" keeps lists in memory only)
	sources    []*source
	client     *http.Client
	startDelay time.Duration // Run's wait before its first update
	snap       atomic.Pointer[snapshot]
	updateMu   sync.Mutex // serializes Update
}

// snapshot is the immutable state lookups read; lists follow DB.sources.
type snapshot struct {
	lists []*list
}

// Open returns a DB with the lists cached under dir/threat, so it works
// at once and offline. A missing or unusable cache just leaves lists empty
// until the next Update. An empty dir keeps everything in memory.
func Open(dir string) *DB {
	return open(dir, defaultSources())
}

func open(dir string, sources []*source) *DB {
	d := &DB{dir: dir, sources: sources, client: newClient(), startDelay: 20 * time.Second}
	d.snap.Store(d.loadCache())
	return d
}

// CheckIP reports whether ip (IPv4 or IPv6, optionally with a zone) is on
// a blocklist. Private, loopback, link-local, CGNAT, multicast and other
// reserved addresses never match.
func (d *DB) CheckIP(ip string) (Match, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return Match{}, false
	}
	a = a.WithZone("").Unmap()
	for _, l := range d.snap.Load().lists {
		if det, ok := l.lookupIP(a); ok {
			return Match{Indicator: a.String(), List: l.src.Name, Category: string(l.src.Category), Detail: det}, true
		}
	}
	return Match{}, false
}

// CheckDomain reports whether a host name or one of its parent domains is
// on a blocklist: a.b.evil.com matches a listed evil.com. A bare top-level
// name such as "com" never matches. An IP literal is checked as with
// CheckIP.
func (d *DB) CheckDomain(name string) (Match, bool) {
	n := normalizeDomain(name)
	if n == "" {
		return Match{}, false
	}
	if _, err := netip.ParseAddr(n); err == nil {
		return d.CheckIP(n)
	}
	if !strings.Contains(n, ".") {
		return Match{}, false
	}
	for _, l := range d.snap.Load().lists {
		if hit, ok := l.lookupDomain(n); ok {
			m := Match{Indicator: n, List: l.src.Name, Category: string(l.src.Category)}
			if hit != n {
				m.Detail = "listed: " + hit
			}
			return m, true
		}
	}
	return Match{}, false
}

// Lists describes every list, in lookup order.
func (d *DB) Lists() []ListInfo {
	s := d.snap.Load()
	out := make([]ListInfo, 0, len(s.lists))
	for _, l := range s.lists {
		out = append(out, ListInfo{
			Name: l.src.Name, URL: l.src.URL, Category: string(l.src.Category),
			Entries: l.entries, Updated: l.updated, Err: l.err,
		})
	}
	return out
}

// Updated returns when a list was last refreshed (downloaded or confirmed
// current), the most recent across all lists; zero if none ever was.
func (d *DB) Updated() time.Time {
	var t time.Time
	for _, l := range d.snap.Load().lists {
		if l.updated.After(t) {
			t = l.updated
		}
	}
	return t
}

// oldest returns the least recent refresh across all lists; zero if any
// list has never been loaded.
func (d *DB) oldest() time.Time {
	var t time.Time
	for i, l := range d.snap.Load().lists {
		if !l.loaded {
			return time.Time{}
		}
		if i == 0 || l.updated.Before(t) {
			t = l.updated
		}
	}
	return t
}
