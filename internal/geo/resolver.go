package geo

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxCache bounds the number of addresses remembered.
const maxCache = 8000

// Resolver looks addresses up in the background: reverse DNS for every
// address, and location for public ones when online lookups are allowed.
// Get never blocks; it returns what is known and queues the rest.
type Resolver struct {
	mu       sync.Mutex
	cache    map[string]*entry
	pendHost []string
	pendGeo  []string
	online   bool
	onChange func()
	path     string
	dirty    bool
	wake     chan struct{}
	domains  func(ip string) (string, bool)
}

// SetDomains sets where domain names for addresses come from (the DNS
// answers apps received).
func (r *Resolver) SetDomains(fn func(ip string) (string, bool)) {
	r.mu.Lock()
	r.domains = fn
	r.mu.Unlock()
}

// withDomain fills in the looked-up domain. Called with r.mu held.
func (r *Resolver) withDomain(in Info) Info {
	if r.domains != nil {
		if d, ok := r.domains(in.IP); ok {
			in.Domain = d
		}
	}
	return in
}

type entry struct {
	Info
	used     time.Time
	geoTried time.Time
}

// New creates a resolver that persists its cache in dir ("" keeps it in
// memory only).
func New(dir string, online bool) *Resolver {
	r := &Resolver{cache: map[string]*entry{}, online: online, wake: make(chan struct{}, 1)}
	if dir != "" {
		r.path = filepath.Join(dir, "geocache.json")
		r.load()
	}
	return r
}

// SetOnline allows or forbids online location lookups.
func (r *Resolver) SetOnline(on bool) {
	r.mu.Lock()
	r.online = on
	r.mu.Unlock()
}

// OnChange sets a function called (from a background goroutine) when new
// information arrives.
func (r *Resolver) OnChange(fn func()) {
	r.mu.Lock()
	r.onChange = fn
	r.mu.Unlock()
}

// Get returns what is known about ip, queueing lookups for the rest.
func (r *Resolver) Get(ip string) Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[ip]
	if !ok {
		e = &entry{Info: Info{IP: ip, Scope: Classify(ip)}}
		r.cache[ip] = e
		if e.Scope != ScopeInvalid && e.Scope != ScopeMulticast {
			r.pendHost = append(r.pendHost, ip)
		}
		if e.Scope == ScopePublic {
			r.pendGeo = append(r.pendGeo, ip)
		}
		r.signal()
	} else if e.Scope == ScopePublic && !e.Located && !e.geoTried.IsZero() && time.Since(e.geoTried) > 10*time.Minute {
		e.geoTried = time.Time{}
		r.pendGeo = append(r.pendGeo, ip)
	}
	e.used = time.Now()
	return r.withDomain(e.Info)
}

// Peek returns what is known without queueing anything.
func (r *Resolver) Peek(ip string) (Info, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[ip]; ok {
		return r.withDomain(e.Info), true
	}
	return r.withDomain(Info{IP: ip, Scope: Classify(ip)}), false
}

func (r *Resolver) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run does the lookups until ctx ends. Reverse DNS runs a few at a time;
// locations are fetched in batches at most every few seconds.
func (r *Resolver) Run(ctx context.Context) {
	sem := make(chan struct{}, 4)
	geoTick := time.NewTicker(4 * time.Second)
	saveTick := time.NewTicker(2 * time.Minute)
	defer geoTick.Stop()
	defer saveTick.Stop()
	defer r.save()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-geoTick.C:
			r.locate(ctx)
		case <-saveTick.C:
			r.save()
			continue
		}
		for _, ip := range r.takeHosts() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func(ip string) {
				defer func() { <-sem }()
				r.reverse(ctx, ip)
			}(ip)
		}
	}
}

func (r *Resolver) takeHosts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ips := r.pendHost
	r.pendHost = nil
	return ips
}

func (r *Resolver) reverse(ctx context.Context, ip string) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	names, _ := net.DefaultResolver.LookupAddr(ctx, ip)
	host := ""
	if len(names) > 0 {
		host = strings.TrimSuffix(names[0], ".")
	}
	r.update(ip, func(e *entry) {
		e.Host = host
		e.HostDone = true
	})
}

func (r *Resolver) locate(ctx context.Context) {
	r.mu.Lock()
	if !r.online {
		// Offline: nothing will locate these, so say so instead of waiting.
		for _, ip := range r.pendGeo {
			if e, ok := r.cache[ip]; ok {
				e.Unlocatable = true
				e.geoTried = time.Now()
			}
		}
		r.pendGeo = nil
	}
	if !r.online || len(r.pendGeo) == 0 {
		r.mu.Unlock()
		return
	}
	n := min(len(r.pendGeo), 50)
	batch := append([]string(nil), r.pendGeo[:n]...)
	r.pendGeo = r.pendGeo[n:]
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	infos, err := lookupBatch(ctx, batch)
	if err != nil {
		// Try again later, but not forever: the next Get after a while.
		now := time.Now()
		for _, ip := range batch {
			r.update(ip, func(e *entry) { e.geoTried, e.Unlocatable = now, true })
		}
		return
	}
	for _, in := range infos {
		in := in
		r.update(in.IP, func(e *entry) {
			e.Country, e.CountryCode, e.Region, e.City = in.Country, in.CountryCode, in.Region, in.City
			e.Org, e.ASN, e.Lat, e.Lon, e.Located = in.Org, in.ASN, in.Lat, in.Lon, in.Located
			e.Unlocatable = !in.Located
		})
	}
}

func (r *Resolver) update(ip string, fn func(*entry)) {
	r.mu.Lock()
	e, ok := r.cache[ip]
	if !ok {
		e = &entry{Info: Info{IP: ip, Scope: Classify(ip)}, used: time.Now()}
		r.cache[ip] = e
	}
	fn(e)
	r.dirty = true
	cb := r.onChange
	r.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (r *Resolver) load() {
	b, err := os.ReadFile(r.path)
	if err != nil {
		return
	}
	var infos []Info
	if json.Unmarshal(b, &infos) != nil {
		return
	}
	now := time.Now()
	for _, in := range infos {
		in.Scope = Classify(in.IP)
		e := &entry{Info: in, used: now}
		if !in.Located {
			e.geoTried = now.Add(-time.Hour) // retry on first use
		}
		r.cache[in.IP] = e
	}
}

func (r *Resolver) save() {
	r.mu.Lock()
	if r.path == "" || !r.dirty {
		r.mu.Unlock()
		return
	}
	r.evict()
	infos := make([]Info, 0, len(r.cache))
	for _, e := range r.cache {
		if e.HostDone || e.Located {
			infos = append(infos, e.Info)
		}
	}
	r.dirty = false
	r.mu.Unlock()
	b, err := json.Marshal(infos)
	if err != nil {
		return
	}
	tmp := r.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, r.path)
	}
}

// evict drops the least recently used entries beyond maxCache. Called
// with r.mu held.
func (r *Resolver) evict() {
	for len(r.cache) > maxCache {
		var oldest string
		var t time.Time
		for ip, e := range r.cache {
			if oldest == "" || e.used.Before(t) {
				oldest, t = ip, e.used
			}
		}
		delete(r.cache, oldest)
	}
}
