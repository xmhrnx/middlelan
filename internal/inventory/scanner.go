package inventory

import (
	"context"
	"sync"
	"time"

	"github.com/xmhrnx/middlelan/internal/nettools"
)

// Scanner scans the local network into an Inventory, on demand and
// periodically in the background. Safe from any goroutine.
type Scanner struct {
	inv *Inventory

	mu      sync.Mutex
	running bool
	done    int
	total   int
	err     error
	cancel  context.CancelFunc

	// OnChange runs (on the scanning goroutine) when progress or results change.
	OnChange func()
	// OnNew runs with devices never seen before, after a scan.
	OnNew func([]Device)
}

// NewScanner creates a scanner for inv.
func NewScanner(inv *Inventory) *Scanner { return &Scanner{inv: inv} }

// Inventory returns the inventory being filled.
func (s *Scanner) Inventory() *Inventory { return s.inv }

// Status reports whether a scan runs, its progress and the last error.
func (s *Scanner) Status() (running bool, done, total int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.done, s.total, s.err
}

// Start begins a scan in the background unless one is running.
func (s *Scanner) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	s.running, s.done, s.total, s.err, s.cancel = true, 0, 0, nil, cancel
	s.mu.Unlock()
	go func() {
		defer cancel()
		s.scan(ctx)
	}()
}

// Stop cancels a running scan.
func (s *Scanner) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

func (s *Scanner) scan(ctx context.Context) {
	found, err := nettools.ScanLAN(ctx, nettools.ScanOptions{}, func(nettools.Device) { s.changed() }, func(done, total int) {
		s.mu.Lock()
		s.done, s.total = done, total
		s.mu.Unlock()
		if done%16 == 0 || done == total {
			s.changed()
		}
	})
	var fresh []Device
	if err == nil || len(found) > 0 {
		devs := make([]Device, 0, len(found))
		for _, d := range found {
			devs = append(devs, Device{
				IP: d.IP, MAC: d.MAC, Hostname: d.Hostname, Vendor: d.Vendor, Kind: d.Kind, OpenPorts: d.OpenPorts,
				IsGateway: d.IsGateway, IsSelf: d.IsSelf, LatencyMs: float64(d.Latency) / float64(time.Millisecond),
			})
		}
		fresh = s.inv.Merge(devs, time.Now())
		if serr := s.inv.Save(); serr != nil && err == nil {
			err = serr
		}
	}
	s.mu.Lock()
	s.running, s.err, s.cancel = false, err, nil
	s.mu.Unlock()
	s.changed()
	if len(fresh) > 0 && s.OnNew != nil {
		s.OnNew(fresh)
	}
}

func (s *Scanner) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// RunPeriodic scans every interval() while enabled() is true, until ctx
// ends. The first scan starts shortly after launch.
func (s *Scanner) RunPeriodic(ctx context.Context, enabled func() bool, interval func() time.Duration) {
	wait := 20 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if enabled() {
			s.Start(ctx)
		}
		wait = interval()
	}
}
