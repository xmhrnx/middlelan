// Package dnslog maps remote IP addresses to the domain names apps looked
// up, by watching DNS answers as they happen: the Windows DNS client cache,
// a kernel-filtered packet capture on Linux and macOS, or answers handed in
// by the Android VPN. Those names (www.youtube.com) are far more telling
// than reverse DNS (a 1e100.net host).
package dnslog

import (
	"container/list"
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// MaxEntries is how many (name, address) pairs a Log keeps; the least
// recently seen pair is dropped first.
const MaxEntries = 20000

// Answer is one observed DNS answer.
type Answer struct {
	// Name is the name the app asked for (the query name, not a CNAME
	// target), lower case and without a trailing dot.
	Name string
	// IP is the address the name resolved to, in canonical text form.
	IP string
	// At is when the answer was observed.
	At time.Time
}

// pair identifies one entry.
type pair struct{ name, ip string }

// entry is one remembered (name, address) pair. seq orders entries by when
// they were last seen; it breaks ties between equal timestamps.
type entry struct {
	Answer
	seq uint64
	el  *list.Element
}

// Log remembers recent DNS answers (bounded to MaxEntries, least recently
// seen first out) and is safe for concurrent use.
type Log struct {
	mu      sync.Mutex
	max     int
	seq     uint64
	byPair  map[pair]*entry
	byIP    map[string][]*entry
	lru     *list.List // front is the most recently seen *entry
	method  string
	added   bool
	running bool
}

// New returns an empty Log.
func New() *Log {
	return newLog(MaxEntries)
}

func newLog(max int) *Log {
	return &Log{
		max:    max,
		byPair: map[pair]*entry{},
		byIP:   map[string][]*entry{},
		lru:    list.New(),
		method: "Not started",
	}
}

// Run captures answers until ctx ends, using the best method this platform
// and privilege level allow; it returns nil when no method is available
// (Method then says why).
func (l *Log) Run(ctx context.Context) error {
	return l.run(ctx)
}

// Lookup returns the most recent name that resolved to ip.
func (l *Log) Lookup(ip string) (name string, ok bool) {
	key, valid := canonicalIP(ip)
	if !valid {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var best *entry
	for _, e := range l.byIP[key] {
		if best == nil || e.seq > best.seq {
			best = e
		}
	}
	if best == nil {
		return "", false
	}
	return best.Name, true
}

// Names returns all names seen for ip, newest first.
func (l *Log) Names(ip string) []string {
	key, valid := canonicalIP(ip)
	if !valid {
		return nil
	}
	l.mu.Lock()
	entries := append([]*entry(nil), l.byIP[key]...)
	l.mu.Unlock()
	if len(entries) == 0 {
		return nil
	}
	// Insertion sort: an address rarely has more than a handful of names.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].seq > entries[j-1].seq; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// Add records an answer from elsewhere (the Android VPN parses DNS itself
// and calls this). Answers with an empty name or an invalid address are
// ignored; a zero At means now.
func (l *Log) Add(a Answer) {
	l.mu.Lock()
	l.added = true
	l.mu.Unlock()
	l.add(a)
}

// add records an answer without marking the Log as fed from elsewhere.
func (l *Log) add(a Answer) {
	ip, ok := canonicalIP(a.IP)
	if !ok {
		return
	}
	name := canonicalName(a.Name)
	if name == "" {
		return
	}
	if a.At.IsZero() {
		a.At = time.Now()
	}
	a.Name, a.IP = name, ip
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	k := pair{name, ip}
	if e, ok := l.byPair[k]; ok {
		e.At = a.At
		e.seq = l.seq
		l.lru.MoveToFront(e.el)
		return
	}
	e := &entry{Answer: a, seq: l.seq}
	e.el = l.lru.PushFront(e)
	l.byPair[k] = e
	l.byIP[ip] = append(l.byIP[ip], e)
	for l.lru.Len() > l.max {
		l.evict(l.lru.Back().Value.(*entry))
	}
}

// evict removes e; l.mu must be held.
func (l *Log) evict(e *entry) {
	l.lru.Remove(e.el)
	delete(l.byPair, pair{e.Name, e.IP})
	es := l.byIP[e.IP]
	for i, x := range es {
		if x == e {
			es[i] = es[len(es)-1]
			es[len(es)-1] = nil
			es = es[:len(es)-1]
			break
		}
	}
	if len(es) == 0 {
		delete(l.byIP, e.IP)
	} else {
		l.byIP[e.IP] = es
	}
}

// Len returns how many (name, address) pairs are remembered.
func (l *Log) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lru.Len()
}

// Method describes how names are captured, for the UI: for example
// "Windows DNS cache", "Packet capture" or "Unavailable: run as
// administrator".
func (l *Log) Method() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.added && !l.running {
		return "Captured by the VPN"
	}
	return l.method
}

// setMethod records how names are captured and whether capture is active.
func (l *Log) setMethod(method string, running bool) {
	l.mu.Lock()
	l.method = method
	l.running = running
	l.mu.Unlock()
}

// canonicalIP parses ip and returns its canonical text form, unmapping
// IPv4-mapped IPv6 addresses and dropping any zone.
func canonicalIP(ip string) (string, bool) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", false
	}
	return addr.Unmap().WithZone("").String(), true
}

// canonicalName lower-cases a DNS name and strips its trailing dot.
func canonicalName(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, "."))
}
