package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// keepDays is how long daily usage is kept.
const keepDays = 90

// maxDestinations bounds the remembered destinations per app.
const maxDestinations = 200

// Destination is a remote address an app has used, remembered across runs.
type Destination struct {
	IP       string    `json:"ip"`
	Port     int       `json:"port"`
	Rx       uint64    `json:"rx"`
	Tx       uint64    `json:"tx"`
	Count    int       `json:"n"`
	LastSeen time.Time `json:"seen"`
}

// DayUsage is one app's traffic on one day.
type DayUsage struct {
	Day string `json:"day"` // 2006-01-02
	ByteCount
}

type storeData struct {
	Usage        map[string]map[string]ByteCount   `json:"usage"`  // day -> app -> bytes
	Destinations map[string]map[string]Destination `json:"dest"`   // app -> ip:port -> destination
	FirstSeen    map[string]time.Time              `json:"first"`  // app -> when first seen
	Hourly       map[string]map[string]ByteCount   `json:"hourly"` // "2006-01-02T15" -> app -> bytes
}

// Store keeps daily usage per app and the destinations apps use in a
// JSON file, written at most once a minute.
type Store struct {
	mu    sync.Mutex
	path  string
	data  storeData
	dirty bool
	saved time.Time
}

// OpenStore loads the store from dir, starting empty when there's no file.
func OpenStore(dir string) *Store {
	s := &Store{data: storeData{
		Usage: map[string]map[string]ByteCount{}, Destinations: map[string]map[string]Destination{}, FirstSeen: map[string]time.Time{},
		Hourly: map[string]map[string]ByteCount{},
	}}
	if dir == "" {
		return s
	}
	s.path = filepath.Join(dir, "usage.json")
	if b, err := os.ReadFile(s.path); err == nil {
		var d storeData
		if json.Unmarshal(b, &d) == nil {
			if d.Usage != nil {
				s.data.Usage = d.Usage
			}
			if d.Destinations != nil {
				s.data.Destinations = d.Destinations
			}
			if d.FirstSeen != nil {
				s.data.FirstSeen = d.FirstSeen
			}
			if d.Hourly != nil {
				s.data.Hourly = d.Hourly
			}
		}
	}
	s.saved = time.Now()
	return s
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

func hourKey(t time.Time) string { return t.Format("2006-01-02T15") }

// keepHours is how long hourly usage is kept.
const keepHours = 14 * 24

// HourUsage is traffic in one hour.
type HourUsage struct {
	Hour time.Time
	ByteCount
}

// Hourly returns the traffic of each of the last n hours, oldest first;
// app "" sums every app.
func (s *Store) Hourly(app string, n int, now time.Time) []HourUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HourUsage, n)
	top := now.Truncate(time.Hour)
	for i := 0; i < n; i++ {
		h := top.Add(time.Duration(i-n+1) * time.Hour)
		out[i].Hour = h
		for a, c := range s.data.Hourly[hourKey(h)] {
			if app == "" || a == app {
				out[i].Rx += c.Rx
				out[i].Tx += c.Tx
			}
		}
	}
	return out
}

// HourApps returns every app's traffic in the hour containing t.
func (s *Store) HourApps(t time.Time) map[string]ByteCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]ByteCount{}
	for k, v := range s.data.Hourly[hourKey(t)] {
		out[k] = v
	}
	return out
}

// Days returns every stored day, oldest first, with each app's traffic.
func (s *Store) Days() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.data.Usage))
	for d := range s.data.Usage {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// Add records traffic for app at time t.
func (s *Store) Add(app string, t time.Time, rx, tx uint64) {
	if rx == 0 && tx == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := dayKey(t)
	m := s.data.Usage[d]
	if m == nil {
		m = map[string]ByteCount{}
		s.data.Usage[d] = m
	}
	c := m[app]
	c.Rx += rx
	c.Tx += tx
	m[app] = c
	addTo(s.data.Hourly, hourKey(t), app, rx, tx)
	s.dirty = true
}

func addTo(m map[string]map[string]ByteCount, key, app string, rx, tx uint64) {
	inner := m[key]
	if inner == nil {
		inner = map[string]ByteCount{}
		m[key] = inner
	}
	c := inner[app]
	c.Rx += rx
	c.Tx += tx
	inner[app] = c
}

// SetToday replaces app's total for today (platforms that report daily
// totals themselves).
func (s *Store) SetToday(app string, t time.Time, c ByteCount) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := dayKey(t)
	m := s.data.Usage[d]
	if m == nil {
		m = map[string]ByteCount{}
		s.data.Usage[d] = m
	}
	if old := m[app]; old != c {
		// The platform reports the day's total; the change since the last
		// report is this hour's share.
		if c.Rx >= old.Rx && c.Tx >= old.Tx {
			addTo(s.data.Hourly, hourKey(t), app, c.Rx-old.Rx, c.Tx-old.Tx)
		}
		m[app] = c
		s.dirty = true
	}
}

// Seen records that app exists, returning true the first time ever.
func (s *Store) Seen(app string, t time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.FirstSeen[app]; ok {
		return false
	}
	s.data.FirstSeen[app] = t
	s.dirty = true
	return true
}

// FirstSeen reports when app was first seen.
func (s *Store) FirstSeen(app string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.FirstSeen[app]
}

// AddDestination records traffic between app and a remote endpoint.
func (s *Store) AddDestination(app, ip string, port int, t time.Time, rx, tx uint64, newConn bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.data.Destinations[app]
	if m == nil {
		m = map[string]Destination{}
		s.data.Destinations[app] = m
	}
	key := ip + "|" + strconv.Itoa(port)
	d := m[key]
	d.IP, d.Port, d.LastSeen = ip, port, t
	d.Rx += rx
	d.Tx += tx
	if newConn {
		d.Count++
	}
	m[key] = d
	if len(m) > maxDestinations {
		var oldest string
		var ot time.Time
		for k, v := range m {
			if oldest == "" || v.LastSeen.Before(ot) {
				oldest, ot = k, v.LastSeen
			}
		}
		delete(m, oldest)
	}
	s.dirty = true
}

// Destinations returns app's remembered destinations, most traffic first.
func (s *Store) Destinations(app string) []Destination {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Destination, 0, len(s.data.Destinations[app]))
	for _, d := range s.data.Destinations[app] {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rx+out[i].Tx > out[j].Rx+out[j].Tx })
	return out
}

// Today returns app's traffic today.
func (s *Store) Today(app string, t time.Time) ByteCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Usage[dayKey(t)][app]
}

// Day returns every app's traffic on the day of t.
func (s *Store) Day(t time.Time) map[string]ByteCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]ByteCount{}
	for k, v := range s.data.Usage[dayKey(t)] {
		out[k] = v
	}
	return out
}

// Daily returns the total traffic of each of the last n days, oldest
// first; app "" sums every app.
func (s *Store) Daily(app string, n int, now time.Time) []DayUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DayUsage, n)
	for i := 0; i < n; i++ {
		d := dayKey(now.AddDate(0, 0, i-n+1))
		out[i].Day = d
		for a, c := range s.data.Usage[d] {
			if app == "" || a == app {
				out[i].Rx += c.Rx
				out[i].Tx += c.Tx
			}
		}
	}
	return out
}

// Apps lists every app ever seen.
func (s *Store) Apps() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.data.FirstSeen))
	for a := range s.data.FirstSeen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Reset forgets all usage and destinations.
func (s *Store) Reset() {
	s.mu.Lock()
	s.data.Usage = map[string]map[string]ByteCount{}
	s.data.Destinations = map[string]map[string]Destination{}
	s.data.Hourly = map[string]map[string]ByteCount{}
	s.dirty = true
	s.mu.Unlock()
	_ = s.Save()
}

// MaybeSave writes the file when it changed and a minute has passed.
func (s *Store) MaybeSave() error {
	s.mu.Lock()
	due := s.dirty && time.Since(s.saved) >= time.Minute
	s.mu.Unlock()
	if !due {
		return nil
	}
	return s.Save()
}

// Save writes the file now, dropping days older than keepDays.
func (s *Store) Save() error {
	s.mu.Lock()
	if s.path == "" {
		s.dirty = false
		s.mu.Unlock()
		return nil
	}
	cutoff := dayKey(time.Now().AddDate(0, 0, -keepDays))
	for d := range s.data.Usage {
		if d < cutoff {
			delete(s.data.Usage, d)
		}
	}
	hourCutoff := hourKey(time.Now().Add(-keepHours * time.Hour))
	for h := range s.data.Hourly {
		if h < hourCutoff {
			delete(s.data.Hourly, h)
		}
	}
	b, err := json.Marshal(s.data)
	s.dirty = false
	s.saved = time.Now()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
