package router

import (
	"context"
	"sync"
	"time"
)

// Point is the whole network's throughput at one moment, bytes per second.
type Point struct {
	At     time.Time
	Rx, Tx float64
}

// Status is what the watcher knows.
type Status struct {
	Found    bool
	Name     string
	Model    string
	RateRx   float64
	RateTx   float64
	LinkDown float64 // Mbps, as the router reports its line
	LinkUp   float64
	History  []Point // oldest first
	Err      string
}

const historyLen = 120

// Watcher polls the router's counters. Safe from any goroutine.
type Watcher struct {
	mu     sync.Mutex
	status Status
	// OnChange runs after every reading, on the watcher's goroutine.
	OnChange func()
}

// Status returns the latest state.
func (w *Watcher) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.status
	s.History = append([]Point(nil), w.status.History...)
	return s
}

// Run finds the router and polls it every interval() until ctx ends.
// When no router answers it retries every 10 minutes, so a router without
// UPnP costs one multicast query now and then.
func (w *Watcher) Run(ctx context.Context, interval func() time.Duration) {
	var gw *Gateway
	var prev Counters
	for {
		wait := interval()
		if gw == nil {
			g, err := Discover(ctx)
			if err != nil {
				w.set(func(s *Status) { s.Found, s.Err = false, err.Error() })
				wait = 10 * time.Minute
			} else {
				gw, prev = g, Counters{}
				w.set(func(s *Status) { s.Found, s.Name, s.Model, s.Err = true, g.Name, g.Model, "" })
			}
		}
		if gw != nil {
			c, err := gw.Counters(ctx)
			if err != nil {
				gw = nil // rediscover: the router may have restarted
				w.set(func(s *Status) { s.Err = err.Error() })
			} else {
				if !prev.At.IsZero() {
					dt := c.At.Sub(prev.At).Seconds()
					if dt > 0 {
						p := Point{At: c.At, Rx: float64(Delta(prev.Rx, c.Rx)) / dt, Tx: float64(Delta(prev.Tx, c.Tx)) / dt}
						w.set(func(s *Status) {
							s.RateRx, s.RateTx, s.LinkDown, s.LinkUp = p.Rx, p.Tx, c.LinkDownMbps, c.LinkUpMbps
							s.History = append(s.History, p)
							if len(s.History) > historyLen {
								s.History = s.History[len(s.History)-historyLen:]
							}
						})
					}
				}
				prev = c
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (w *Watcher) set(fn func(*Status)) {
	w.mu.Lock()
	fn(&w.status)
	cb := w.OnChange
	w.mu.Unlock()
	if cb != nil {
		cb()
	}
}
