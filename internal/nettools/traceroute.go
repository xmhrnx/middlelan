package nettools

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Hop is one row of a traceroute.
type Hop struct {
	// TTL is the hop number, starting at 1.
	TTL int
	// Addr is the address of the router (or destination) that answered and
	// Host its reverse DNS name, if any.
	Addr, Host string
	// RTT is the round-trip time to this hop. With the system ping fallback
	// it is approximate when the tool does not print a time.
	RTT time.Duration
	// Timeout is true when nothing answered for this TTL.
	Timeout bool
	// Reached is true when the destination itself answered.
	Reached bool
}

const (
	defaultMaxHops   = 30
	maxMaxHops       = 64
	traceParallelism = 8
	hopTimeout       = time.Second
)

// hopProber probes a single TTL. A non-nil error means the technique cannot
// be used at all; a silent hop is reported via Hop.Timeout.
type hopProber func(ctx context.Context, ttl int) (Hop, error)

// systemHopProber probes with the system ping binary limited to ttl hops.
func systemHopProber(ip net.IP) hopProber {
	return func(ctx context.Context, ttl int) (Hop, error) {
		hop := Hop{TTL: ttl}
		res, err := runSystemPing(ctx, ip, ttl, hopTimeout)
		if err != nil {
			return hop, err
		}
		switch {
		case res.TTLExceeded && res.From != "":
			hop.Addr, hop.RTT = res.From, res.RTT
		case res.Reply:
			hop.Addr, hop.RTT, hop.Reached = res.From, res.RTT, true
			if hop.Addr == "" {
				hop.Addr = ip.String()
			}
		default:
			hop.Timeout = true
		}
		return hop, nil
	}
}

// Traceroute discovers the routers between this machine and host, calling
// onHop (if non-nil) for every hop in TTL order. maxHops <= 0 defaults to 30.
// Up to 8 TTLs are probed in parallel; probing stops once the destination
// has answered. On Windows the native IcmpSendEcho API is used for IPv4;
// elsewhere (and for IPv6 on Windows) each TTL runs the system ping binary.
func Traceroute(ctx context.Context, host string, maxHops int, onHop func(Hop)) ([]Hop, error) {
	ip, err := resolveIP(ctx, host)
	if err != nil {
		return nil, err
	}
	if maxHops <= 0 {
		maxHops = defaultMaxHops
	}
	if maxHops > maxMaxHops {
		maxHops = maxMaxHops
	}
	probe := platformHopProber(ip)
	if probe == nil {
		if _, err := newExecPinger(ip); err != nil {
			return nil, err
		}
		probe = systemHopProber(ip)
	}

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		hop Hop
		err error
	}
	ch := make(chan result)
	var reached atomic.Int64
	reached.Store(int64(maxHops))

	var wg sync.WaitGroup
	go func() {
		sem := make(chan struct{}, traceParallelism)
	launch:
		for ttl := 1; ttl <= maxHops; ttl++ {
			select {
			case sem <- struct{}{}:
			case <-ctx2.Done():
				break launch
			}
			if int64(ttl) > reached.Load() {
				<-sem
				break
			}
			wg.Add(1)
			go func(ttl int) {
				defer wg.Done()
				hop, err := probe(ctx2, ttl)
				hop.TTL = ttl
				if err == nil && !hop.Timeout && hop.Addr != "" {
					hop.Host = reverseLookup(ctx2, hop.Addr)
				}
				<-sem
				ch <- result{hop, err}
			}(ttl)
		}
		wg.Wait()
		close(ch)
	}()

	pending := make(map[int]result)
	var hops []Hop
	var firstErr error
	next := 1
	last := maxHops
	for r := range ch {
		if r.hop.TTL > last {
			continue // beyond the destination, or cancelled
		}
		if r.err == nil && r.hop.Reached && r.hop.TTL < last {
			last = r.hop.TTL
			reached.Store(int64(last))
		}
		pending[r.hop.TTL] = r
		for next <= last && firstErr == nil {
			pr, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			if pr.err != nil {
				firstErr = pr.err
				cancel()
				break
			}
			hops = append(hops, pr.hop)
			if onHop != nil {
				onHop(pr.hop)
			}
			next++
		}
		if next > last {
			cancel() // done: stop outstanding probes
		}
	}
	if firstErr != nil {
		if ctx.Err() != nil {
			return hops, ctx.Err()
		}
		return hops, firstErr
	}
	if ctx.Err() != nil {
		return hops, ctx.Err()
	}
	return hops, nil
}
