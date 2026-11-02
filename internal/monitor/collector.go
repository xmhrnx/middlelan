package monitor

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	historyLen    = 180 // whole-computer rate points kept
	appHistoryLen = 60  // per-app rate points kept
	maxEndpoints  = 300 // live endpoints kept per app
)

// Options configures a Collector.
type Options struct {
	// Interval between samples while the window is shown, and while it's
	// hidden (sampling slows down in the background to save power).
	Interval, HiddenInterval time.Duration
	// DataDir is where usage is saved ("" keeps it in memory).
	DataDir string
	// OnUpdate runs after every sample, on the collector's goroutine.
	OnUpdate func()
	// OnNewApp runs when a program uses the network for the first time
	// ever, on the collector's goroutine.
	OnNewApp func(name string)
}

// Collector samples a Source on its own goroutine and keeps an aggregated
// Snapshot for the UI. All methods are safe from any goroutine.
type Collector struct {
	src   Source
	opts  Options
	store *Store

	snap    atomic.Pointer[Snapshot]
	visible atomic.Bool
	paused  atomic.Bool
	fast    atomic.Int64 // visible interval override, nanoseconds; 0 uses Options
	kick    chan struct{}

	// Sampling state, owned by the collector goroutine.
	prevTime  time.Time
	prevIface ByteCount
	prevSock  map[string]ByteCount
	prevProc  map[int]ByteCount
	prevApp   map[string]ByteCount
	apps      map[string]*appState
	history   []RatePoint
	totalRx   uint64
	totalTx   uint64
	samples   int
	mu        sync.Mutex // guards apps' endpoints for EndpointsOf
}

type appState struct {
	name, exe            string
	sessionRx, sessionTx uint64
	rateRx, rateTx       float64
	endpoints            map[string]*Endpoint
	firstSeen, lastSeen  time.Time
	hist                 []float64
	knownBytes           bool
}

// NewCollector creates a collector; call Run to start it.
func NewCollector(src Source, opts Options) *Collector {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	if opts.HiddenInterval <= 0 {
		opts.HiddenInterval = 6 * time.Second
	}
	c := &Collector{
		src: src, opts: opts, store: OpenStore(opts.DataDir), kick: make(chan struct{}, 1),
		prevSock: map[string]ByteCount{}, prevProc: map[int]ByteCount{}, prevApp: map[string]ByteCount{},
		apps: map[string]*appState{},
	}
	c.visible.Store(true)
	c.snap.Store(&Snapshot{Time: time.Now(), Limits: src.Limits(), Interval: opts.Interval})
	return c
}

// Store returns the persistent usage store.
func (c *Collector) Store() *Store { return c.store }

// Snapshot returns the latest aggregated view. It's immutable: the
// collector replaces it rather than changing it.
func (c *Collector) Snapshot() *Snapshot { return c.snap.Load() }

// SetVisible tells the collector whether the window is shown, which picks
// the sampling interval.
func (c *Collector) SetVisible(v bool) {
	if c.visible.Swap(v) != v && v {
		c.Refresh()
	}
}

// SetPaused stops or resumes sampling.
func (c *Collector) SetPaused(p bool) {
	c.paused.Store(p)
	if !p {
		c.Refresh()
	}
}

// SetInterval changes the sampling interval while the window is shown.
func (c *Collector) SetInterval(d time.Duration) {
	c.fast.Store(int64(d))
	c.Refresh()
}

func (c *Collector) interval() time.Duration {
	if !c.visible.Load() {
		return c.opts.HiddenInterval
	}
	if d := c.fast.Load(); d > 0 {
		return time.Duration(d)
	}
	return c.opts.Interval
}

// Paused reports whether sampling is paused.
func (c *Collector) Paused() bool { return c.paused.Load() }

// Refresh takes a sample now.
func (c *Collector) Refresh() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// Run samples until ctx ends, then saves the store.
func (c *Collector) Run(ctx context.Context) {
	defer c.store.Save() //nolint:errcheck // best effort at exit; nowhere to report
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-c.kick:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		if !c.paused.Load() {
			c.sample()
		}
		timer.Reset(c.interval())
	}
}

func counterDelta(cur, prev uint64) uint64 {
	if cur < prev {
		return 0 // counter reset
	}
	return cur - prev
}

func (c *Collector) sample() {
	now := time.Now()
	smp, err := c.src.Sample()
	if err != nil {
		old := c.snap.Load()
		next := *old
		next.Err = err.Error()
		next.Time = now
		c.snap.Store(&next)
		c.notify()
		return
	}
	first := c.samples == 0
	c.samples++
	dt := now.Sub(c.prevTime).Seconds()
	if first || dt <= 0 {
		dt = c.interval().Seconds()
	}

	var rateRx, rateTx float64
	if !first {
		drx, dtx := counterDelta(smp.Iface.Rx, c.prevIface.Rx), counterDelta(smp.Iface.Tx, c.prevIface.Tx)
		c.totalRx += drx
		c.totalTx += dtx
		rateRx, rateTx = float64(drx)/dt, float64(dtx)/dt
	}
	c.prevIface = smp.Iface
	c.prevTime = now

	deltas := c.attribute(smp, now, first)
	for _, a := range c.apps {
		d := deltas[a.name]
		a.rateRx, a.rateTx = float64(d.Rx)/dt, float64(d.Tx)/dt
		a.sessionRx += d.Rx
		a.sessionTx += d.Tx
		a.hist = appendCapped(a.hist, a.rateRx+a.rateTx, appHistoryLen)
		if smp.AppTotals == nil {
			c.store.Add(a.name, now, d.Rx, d.Tx)
		}
	}
	c.history = append(c.history, RatePoint{Time: now, Rx: rateRx, Tx: rateTx})
	if len(c.history) > historyLen {
		c.history = c.history[len(c.history)-historyLen:]
	}
	c.publish(smp, now, rateRx, rateTx)
	_ = c.store.MaybeSave() // a failed save is retried next minute
	c.notify()
}

// attribute assigns this sample's connections and byte deltas to apps and
// returns each app's bytes in this interval.
func (c *Collector) attribute(smp Sample, now time.Time, first bool) map[string]ByteCount {
	deltas := map[string]ByteCount{}
	seenSock := make(map[string]ByteCount, len(smp.Conns))
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.apps {
		for _, e := range a.endpoints {
			e.Active = false
		}
	}
	for _, conn := range smp.Conns {
		a := c.app(conn.Process, conn.Exe, now)
		a.lastSeen = now
		var d ByteCount
		if conn.HasBytes {
			a.knownBytes = true
			cur := ByteCount{Rx: conn.Recv, Tx: conn.Sent}
			seenSock[conn.Key] = cur
			if prev, ok := c.prevSock[conn.Key]; ok {
				d = ByteCount{Rx: counterDelta(cur.Rx, prev.Rx), Tx: counterDelta(cur.Tx, prev.Tx)}
			} else if !first {
				d = cur // opened since the last sample
			}
			t := deltas[a.name]
			t.Rx += d.Rx
			t.Tx += d.Tx
			deltas[a.name] = t
		}
		if conn.Listening() {
			continue
		}
		key := conn.Proto + "|" + conn.RemoteIP + "|" + itoa(conn.RemotePort)
		e, ok := a.endpoints[key]
		if !ok {
			e = &Endpoint{IP: conn.RemoteIP, Port: conn.RemotePort, Proto: conn.Proto, FirstSeen: now}
			a.endpoints[key] = e
			trimEndpoints(a.endpoints)
		}
		e.Active, e.LastSeen = !conn.Closed(), now
		e.Rx += d.Rx
		e.Tx += d.Tx
		_, known := c.prevSock[conn.Key]
		if d.Rx+d.Tx > 0 || !ok {
			c.store.AddDestination(a.name, conn.RemoteIP, conn.RemotePort, now, d.Rx, d.Tx, !known && !ok)
		}
	}
	c.prevSock = seenSock

	if smp.Proc != nil {
		seen := make(map[int]ByteCount, len(smp.Proc))
		for pid, p := range smp.Proc {
			seen[pid] = p.ByteCount
			prev, ok := c.prevProc[pid]
			if !ok && first {
				continue
			}
			d := ByteCount{Rx: counterDelta(p.Rx, prev.Rx), Tx: counterDelta(p.Tx, prev.Tx)}
			if d.Rx+d.Tx == 0 {
				continue
			}
			a := c.app(p.Name, p.Exe, now)
			a.knownBytes, a.lastSeen = true, now
			t := deltas[a.name]
			t.Rx += d.Rx
			t.Tx += d.Tx
			deltas[a.name] = t
		}
		c.prevProc = seen
	}

	if smp.AppTotals != nil {
		for name, tot := range smp.AppTotals {
			a := c.app(name, "", now)
			a.knownBytes = true
			if prev, ok := c.prevApp[name]; ok {
				d := ByteCount{Rx: counterDelta(tot.Rx, prev.Rx), Tx: counterDelta(tot.Tx, prev.Tx)}
				if d.Rx+d.Tx > 0 {
					a.lastSeen = now
				}
				t := deltas[name]
				t.Rx += d.Rx
				t.Tx += d.Tx
				deltas[name] = t
			}
			c.store.SetToday(name, now, tot)
		}
		c.prevApp = smp.AppTotals
	}
	return deltas
}

func (c *Collector) app(name, exe string, now time.Time) *appState {
	if name == "" {
		name = "Unknown"
	}
	a, ok := c.apps[name]
	if !ok {
		a = &appState{name: name, exe: exe, endpoints: map[string]*Endpoint{}, firstSeen: now}
		c.apps[name] = a
		if c.store.Seen(name, now) && c.samples > 1 && c.opts.OnNewApp != nil {
			c.opts.OnNewApp(name)
		}
	}
	if a.exe == "" {
		a.exe = exe
	}
	return a
}

func trimEndpoints(m map[string]*Endpoint) {
	for len(m) > maxEndpoints {
		var oldest string
		var t time.Time
		for k, e := range m {
			if !e.Active && (oldest == "" || e.LastSeen.Before(t)) {
				oldest, t = k, e.LastSeen
			}
		}
		if oldest == "" {
			return
		}
		delete(m, oldest)
	}
}

func appendCapped(s []float64, v float64, n int) []float64 {
	s = append(s, v)
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

// publish builds a fresh immutable snapshot.
func (c *Collector) publish(smp Sample, now time.Time, rateRx, rateTx float64) {
	byApp := map[string][]Conn{}
	for _, conn := range smp.Conns {
		name := conn.Process
		if name == "" {
			name = "Unknown"
		}
		byApp[name] = append(byApp[name], conn)
	}
	today := c.store.Day(now)
	var todayRx, todayTx uint64
	for _, v := range today {
		todayRx += v.Rx
		todayTx += v.Tx
	}
	c.mu.Lock()
	apps := make([]App, 0, len(c.apps))
	for _, a := range c.apps {
		conns := byApp[a.name]
		if len(conns) == 0 && a.rateRx+a.rateTx == 0 && now.Sub(a.lastSeen) > 10*time.Minute && today[a.name] == (ByteCount{}) {
			continue
		}
		app := App{
			Name: a.name, Exe: a.exe, Conns: conns, RateRx: a.rateRx, RateTx: a.rateTx,
			SessionRx: a.sessionRx, SessionTx: a.sessionTx, TodayRx: today[a.name].Rx, TodayTx: today[a.name].Tx,
			FirstSeen: c.store.FirstSeen(a.name), LastSeen: a.lastSeen,
			History: append([]float64(nil), a.hist...), KnownBytes: a.knownBytes,
		}
		pids := map[int]bool{}
		for _, conn := range conns {
			if conn.PID > 0 && !pids[conn.PID] {
				pids[conn.PID] = true
				app.PIDs = append(app.PIDs, conn.PID)
			}
			switch {
			case conn.Listening():
				app.Listening++
			case !conn.Closed():
				app.Active++
			}
		}
		sort.Ints(app.PIDs)
		app.Endpoints = make([]Endpoint, 0, len(a.endpoints))
		for _, e := range a.endpoints {
			app.Endpoints = append(app.Endpoints, *e)
		}
		sort.Slice(app.Endpoints, func(i, j int) bool {
			ei, ej := app.Endpoints[i], app.Endpoints[j]
			if ei.Active != ej.Active {
				return ei.Active
			}
			if ei.Rx+ei.Tx != ej.Rx+ej.Tx {
				return ei.Rx+ei.Tx > ej.Rx+ej.Tx
			}
			return ei.LastSeen.After(ej.LastSeen)
		})
		apps = append(apps, app)
	}
	c.mu.Unlock()
	sort.Slice(apps, func(i, j int) bool {
		ri, rj := apps[i].RateRx+apps[i].RateTx, apps[j].RateRx+apps[j].RateTx
		if ri != rj {
			return ri > rj
		}
		ti, tj := apps[i].TodayRx+apps[i].TodayTx, apps[j].TodayRx+apps[j].TodayTx
		if ti != tj {
			return ti > tj
		}
		if apps[i].Active != apps[j].Active {
			return apps[i].Active > apps[j].Active
		}
		return apps[i].Name < apps[j].Name
	})
	interval := c.interval()
	c.snap.Store(&Snapshot{
		Time: now, Apps: apps, Conns: smp.Conns, RateRx: rateRx, RateTx: rateTx,
		TotalRx: c.totalRx, TotalTx: c.totalTx, TodayRx: todayRx, TodayTx: todayTx,
		History: append([]RatePoint(nil), c.history...), PerApp: smp.PerApp,
		Limits: c.src.Limits(), Interval: interval,
	})
}

func (c *Collector) notify() {
	if c.opts.OnUpdate != nil {
		c.opts.OnUpdate()
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
