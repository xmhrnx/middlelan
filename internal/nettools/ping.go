package nettools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"time"
)

// PingReply describes the outcome of a single echo request.
type PingReply struct {
	// Seq is the zero-based sequence number of the request.
	Seq int
	// Addr is the address that answered (or was probed).
	Addr string
	// RTT is the measured round-trip time when OK is true.
	RTT time.Duration
	// TTL is the TTL / hop limit of the reply, or 0 when unknown.
	TTL int
	// OK reports whether a reply was received.
	OK bool
	// Err holds a human-readable reason when OK is false.
	Err string
}

// PingStats summarises a ping run.
type PingStats struct {
	// Host is the host as given by the caller, Addr the resolved address.
	Host, Addr string
	// Sent and Received count echo requests and replies.
	Sent, Received int
	// Min, Avg, Max and Jitter are computed over successful replies. Jitter
	// is the mean absolute difference between consecutive RTTs.
	Min, Avg, Max, Jitter time.Duration
	// Loss is the fraction of lost requests, 0..1.
	Loss float64
	// Method is the probing technique that was used: "icmp", "system" or
	// "tcp".
	Method string
}

const (
	pingTimeout     = 2 * time.Second
	defaultInterval = time.Second
)

// pinger is one probing technique. ping returns a non-nil error only when the
// technique itself is unusable (permission denied, binary missing, ...);
// an unanswered request is reported through PingReply.OK.
type pinger interface {
	method() string
	ping(ctx context.Context, seq int, timeout time.Duration) (PingReply, error)
	close() error
}

type pingerFactory func(ip net.IP) (pinger, error)

// pingerFactories returns the probing techniques in order of preference.
func pingerFactories() []pingerFactory {
	var f []pingerFactory
	// Windows has no datagram ICMP sockets and raw-socket receive is
	// unreliable there; the native IcmpSendEcho API is used instead.
	if runtime.GOOS != "windows" {
		f = append(f,
			func(ip net.IP) (pinger, error) { return newICMPPinger(ip, false) },
			func(ip net.IP) (pinger, error) { return newICMPPinger(ip, true) },
		)
	}
	f = append(f, platformPingers()...)
	f = append(f,
		func(ip net.IP) (pinger, error) { return newExecPinger(ip) },
		func(ip net.IP) (pinger, error) { return newTCPPinger(ip), nil },
	)
	return f
}

// Ping sends count echo requests to host, one every interval, calling onReply
// (if non-nil) for each result. count <= 0 runs until ctx is cancelled, in
// which case cancellation is the normal way to stop and the error is nil.
// When count > 0 and ctx is cancelled early, the partial statistics are
// returned together with ctx.Err().
//
// Techniques are tried in order: unprivileged ICMP, raw ICMP, the native
// Windows ICMP API (IcmpSendEcho), the system ping binary and finally TCP
// connect timing to port 443 or 80.
func Ping(ctx context.Context, host string, count int, interval time.Duration, onReply func(PingReply)) (PingStats, error) {
	stats := PingStats{Host: host}
	ip, err := resolveIP(ctx, host)
	if err != nil {
		return stats, err
	}
	stats.Addr = ip.String()
	if interval <= 0 {
		interval = defaultInterval
	}

	factories := pingerFactories()
	next := 0
	var p pinger
	openNext := func() error {
		var errs []error
		for next < len(factories) {
			f := factories[next]
			next++
			np, err := f(ip)
			if err == nil {
				p = np
				stats.Method = np.method()
				return nil
			}
			errs = append(errs, err)
		}
		return fmt.Errorf("ping: no usable method: %w", errors.Join(errs...))
	}
	if err := openNext(); err != nil {
		return stats, err
	}
	defer func() {
		if p != nil {
			_ = p.close() // best-effort cleanup; nothing useful to report
		}
	}()

	var rtts []time.Duration
	methodWorked := false
	for seq := 0; count <= 0 || seq < count; seq++ {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		reply, perr := p.ping(ctx, seq, pingTimeout)
		if ctx.Err() != nil {
			break // in-flight request interrupted: do not count it
		}
		if perr != nil {
			if !methodWorked {
				if cerr := p.close(); cerr != nil {
					return finishStats(stats, rtts), cerr
				}
				p = nil
				if err := openNext(); err != nil {
					return finishStats(stats, rtts), err
				}
				seq--
				continue
			}
			reply = PingReply{Seq: seq, Addr: stats.Addr, Err: perr.Error()}
		} else {
			methodWorked = true
		}
		reply.Seq = seq
		if reply.Addr == "" {
			reply.Addr = stats.Addr
		}
		stats.Sent++
		if reply.OK {
			stats.Received++
			rtts = append(rtts, reply.RTT)
		} else if reply.Err == "" {
			reply.Err = "request timed out"
		}
		if onReply != nil {
			onReply(reply)
		}
		if count > 0 && seq == count-1 {
			break
		}
		if err := sleepCtx(ctx, interval-time.Since(start)); err != nil {
			break
		}
	}
	stats = finishStats(stats, rtts)
	if count > 0 && stats.Sent < count && ctx.Err() != nil {
		return stats, ctx.Err()
	}
	return stats, nil
}

// finishStats fills the derived fields of s from the successful RTTs.
func finishStats(s PingStats, rtts []time.Duration) PingStats {
	if s.Sent > 0 {
		s.Loss = 1 - float64(s.Received)/float64(s.Sent)
	}
	if len(rtts) == 0 {
		return s
	}
	var sum, jsum time.Duration
	s.Min, s.Max = rtts[0], rtts[0]
	for i, r := range rtts {
		sum += r
		if r < s.Min {
			s.Min = r
		}
		if r > s.Max {
			s.Max = r
		}
		if i > 0 {
			d := r - rtts[i-1]
			if d < 0 {
				d = -d
			}
			jsum += d
		}
	}
	s.Avg = sum / time.Duration(len(rtts))
	if len(rtts) > 1 {
		s.Jitter = jsum / time.Duration(len(rtts)-1)
	}
	return s
}
