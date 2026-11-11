// Package adblock blocks ads and trackers by domain name, from open
// source blocklists, for the on-device VPN's DNS filter: a query for a
// listed name is answered on the device and never leaves it.
package adblock

import (
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Source is one downloadable blocklist.
type Source struct {
	ID       string
	Name     string
	URL      string
	Homepage string
	License  string
	// MinEntries rejects a download with fewer usable entries: a
	// truncated or changed file rather than the real list.
	MinEntries int
}

// Sources are the built-in lists, all open source and free to use.
var Sources = []Source{
	{
		ID: "adguard-dns", Name: "AdGuard DNS filter",
		URL:      "https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt",
		Homepage: "https://github.com/AdguardTeam/AdGuardSDNSFilter", License: "GPL-3.0",
		MinEntries: 10000,
	},
	{
		ID: "stevenblack", Name: "StevenBlack Unified hosts",
		URL:      "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
		Homepage: "https://github.com/StevenBlack/hosts", License: "MIT",
		MinEntries: 10000,
	},
}

// ListInfo describes the state of one list.
type ListInfo struct {
	Source
	Entries int
	Updated time.Time
	Err     string
}

// Event is one blocked query.
type Event struct {
	At   time.Time
	App  string
	Name string
}

// Count is a name with how often it was blocked.
type Count struct {
	Name  string
	Count int
}

// Stats summarizes what was blocked since MiddleMan started.
type Stats struct {
	Today, Total int
	TopDomains   []Count // today, most blocked first
	TopApps      []Count // today
	Recent       []Event // newest first
}

const (
	recentMax = 200
	topMax    = 50
)

// Blocker matches names against the lists and keeps the statistics.
type Blocker struct {
	dir     string
	sources []Source
	client  httpDoer

	set   atomic.Pointer[rules] // all lists merged
	allow atomic.Pointer[map[string]bool]

	updateMu sync.Mutex
	listsMu  sync.Mutex
	lists    []*list // per source, guarded by listsMu

	statsMu sync.Mutex
	day     string
	today   int
	total   int
	domains map[string]int
	apps    map[string]int
	recent  []Event // ring, oldest first
}

// Open loads the cached lists from dir/adblock; "" keeps nothing on disk.
func Open(dir string) *Blocker {
	b := &Blocker{dir: dir, sources: Sources, client: newClient(), domains: map[string]int{}, apps: map[string]int{}}
	b.allow.Store(&map[string]bool{})
	b.lists = b.loadCache()
	b.rebuild()
	return b
}

// SetAllowed sets the names the user never wants blocked (with their
// subdomains).
func (b *Blocker) SetAllowed(names []string) {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		if n = normalize(n); n != "" {
			m[n] = true
		}
	}
	b.allow.Store(&m)
}

// Blocked reports whether a query for name should be refused.
func (b *Blocker) Blocked(name string) bool {
	name = normalize(name)
	if name == "" {
		return false
	}
	allow := *b.allow.Load()
	for s := name; s != ""; s = parent(s) {
		if allow[s] {
			return false
		}
	}
	return b.set.Load().match(name)
}

// Record counts a blocked query.
func (b *Blocker) Record(app, name string) {
	now := time.Now()
	day := now.Format(time.DateOnly)
	if app == "" {
		app = "Android"
	}
	b.statsMu.Lock()
	defer b.statsMu.Unlock()
	if day != b.day {
		b.day, b.today = day, 0
		clear(b.domains)
		clear(b.apps)
	}
	b.today++
	b.total++
	b.domains[name]++
	b.apps[app]++
	if len(b.recent) == recentMax {
		b.recent = b.recent[1:]
	}
	b.recent = append(b.recent, Event{At: now, App: app, Name: name})
}

// Stats returns a copy of the statistics.
func (b *Blocker) Stats() Stats {
	b.statsMu.Lock()
	defer b.statsMu.Unlock()
	st := Stats{Total: b.total}
	if b.day == time.Now().Format(time.DateOnly) {
		st.Today = b.today
		st.TopDomains = top(b.domains)
		st.TopApps = top(b.apps)
	}
	st.Recent = make([]Event, len(b.recent))
	for i, e := range b.recent {
		st.Recent[len(b.recent)-1-i] = e
	}
	return st
}

func top(m map[string]int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	slices.SortFunc(out, func(a, b Count) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out[:min(len(out), topMax)]
}

// Lists describes every list.
func (b *Blocker) Lists() []ListInfo {
	b.listsMu.Lock()
	defer b.listsMu.Unlock()
	out := make([]ListInfo, len(b.lists))
	for i, l := range b.lists {
		out[i] = ListInfo{Source: b.sources[i], Entries: l.rules.size(), Updated: l.updated, Err: l.err}
	}
	return out
}

// Domains is how many names the merged lists hold.
func (b *Blocker) Domains() int { return b.set.Load().size() }

// Run updates the lists now and then every interval while enabled says
// so, until ctx ends. Lists cached less than interval ago aren't
// downloaded again at startup.
func (b *Blocker) Run(ctx context.Context, every time.Duration, enabled func() bool) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if enabled() && b.stale(every) {
			_ = b.Update(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// stale reports whether a list is missing or older than every; after a
// failure it waits an hour before trying again.
func (b *Blocker) stale(every time.Duration) bool {
	b.listsMu.Lock()
	defer b.listsMu.Unlock()
	for _, l := range b.lists {
		age := time.Since(l.updated)
		if l.err != "" && time.Since(l.tried) < time.Hour {
			continue
		}
		if l.updated.IsZero() || age > every {
			return true
		}
	}
	return false
}

// rebuild merges the lists into the set Blocked uses.
func (b *Blocker) rebuild() {
	b.listsMu.Lock()
	parts := make([]*rules, len(b.lists))
	for i, l := range b.lists {
		parts[i] = l.rules
	}
	b.listsMu.Unlock()
	b.set.Store(merge(parts))
}

func normalize(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

func parent(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}
