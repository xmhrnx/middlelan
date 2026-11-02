// Package alerts watches connections for known-malicious destinations and
// remembers what it found, so the user can see which app talked to what.
package alerts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/internal/threat"
)

// Alert is an app reaching a listed address or domain.
type Alert struct {
	App       string    `json:"app"`
	IP        string    `json:"ip"`
	Port      int       `json:"port"`
	Domain    string    `json:"domain,omitempty"`
	List      string    `json:"list"`
	Category  string    `json:"category"`
	Detail    string    `json:"detail,omitempty"`
	FirstSeen time.Time `json:"first"`
	LastSeen  time.Time `json:"last"`
	Count     int       `json:"count"`
	Dismissed bool      `json:"dismissed,omitempty"`
}

// Key identifies an alert: one per app and indicator.
func (a Alert) Key() string { return a.App + "|" + a.IP + "|" + a.Domain }

const maxAlerts = 500

// Watcher checks connections against the threat lists. Safe from any
// goroutine.
type Watcher struct {
	db      *threat.DB
	domains func(ip string) (string, bool)
	path    string

	mu      sync.Mutex
	alerts  map[string]*Alert
	checked map[string]time.Time // ip -> last checked, so each address is checked once per list update
}

// New creates a watcher saving its alerts in dir.
func New(db *threat.DB, domains func(ip string) (string, bool), dir string) *Watcher {
	w := &Watcher{db: db, domains: domains, alerts: map[string]*Alert{}, checked: map[string]time.Time{}}
	if dir != "" {
		w.path = filepath.Join(dir, "alerts.json")
		if b, err := os.ReadFile(w.path); err == nil {
			var list []*Alert
			if json.Unmarshal(b, &list) == nil {
				for _, a := range list {
					w.alerts[a.Key()] = a
				}
			}
		}
	}
	return w
}

// Check looks at the sample's connections and returns alerts seen for
// the first time.
func (w *Watcher) Check(conns []monitor.Conn, now time.Time) []Alert {
	updated := w.db.Updated()
	var fresh []Alert
	w.mu.Lock()
	defer w.mu.Unlock()
	changed := false
	for _, c := range conns {
		if c.Listening() {
			continue
		}
		domain := ""
		if w.domains != nil {
			domain, _ = w.domains(c.RemoteIP)
		}
		ck := c.RemoteIP + "|" + domain
		if t, ok := w.checked[ck]; ok && !t.Before(updated) {
			// Seen and clean since the lists last changed; an existing
			// alert still gets its last-seen time below.
			if a := w.alerts[c.Process+"|"+c.RemoteIP+"|"+domain]; a != nil && now.Sub(a.LastSeen) > time.Minute {
				a.LastSeen, a.Count = now, a.Count+1
				changed = true
			}
			continue
		}
		w.checked[ck] = now
		m, ok := w.db.CheckIP(c.RemoteIP)
		if !ok && domain != "" {
			m, ok = w.db.CheckDomain(domain)
		}
		if !ok {
			continue
		}
		a := Alert{App: c.Process, IP: c.RemoteIP, Port: c.RemotePort, Domain: domain, List: m.List, Category: m.Category, Detail: m.Detail, FirstSeen: now, LastSeen: now, Count: 1}
		if old := w.alerts[a.Key()]; old != nil {
			old.LastSeen, old.Count = now, old.Count+1
		} else {
			w.alerts[a.Key()] = &a
			fresh = append(fresh, a)
		}
		changed = true
	}
	if len(w.checked) > 50000 {
		w.checked = map[string]time.Time{}
	}
	if changed {
		w.trim()
		w.saveLocked()
	}
	return fresh
}

// List returns the alerts, newest first.
func (w *Watcher) List() []Alert {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Alert, 0, len(w.alerts))
	for _, a := range w.alerts {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

// Dismiss marks an alert as seen.
func (w *Watcher) Dismiss(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if a := w.alerts[key]; a != nil {
		a.Dismissed = true
		w.saveLocked()
	}
}

// Active counts alerts not dismissed.
func (w *Watcher) Active() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, a := range w.alerts {
		if !a.Dismissed {
			n++
		}
	}
	return n
}

func (w *Watcher) trim() {
	for len(w.alerts) > maxAlerts {
		var oldest string
		var t time.Time
		for k, a := range w.alerts {
			if oldest == "" || a.LastSeen.Before(t) {
				oldest, t = k, a.LastSeen
			}
		}
		delete(w.alerts, oldest)
	}
}

func (w *Watcher) saveLocked() {
	if w.path == "" {
		return
	}
	list := make([]*Alert, 0, len(w.alerts))
	for _, a := range w.alerts {
		list = append(list, a)
	}
	b, err := json.Marshal(list)
	if err != nil {
		return
	}
	tmp := w.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, w.path) // retried on the next change
	}
}
