package vpn

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var bufPool = sync.Pool{New: func() any { b := make([]byte, copyBufSize); return &b }}

// halfCloser is a connection whose sending side can be closed alone.
type halfCloser interface{ CloseWrite() error }

// spliceTCP copies between the app's connection and the real one until
// both directions end, counting bytes on f. When one direction ends, the
// other is half-closed and must not stay idle longer than halfCloseIdle.
func (e *Engine) spliceTCP(f *flow, app, remote net.Conn) {
	var half atomic.Bool
	var dns *dnsStream
	if f.RemotePort == 53 && e.hooks.Load().DNS != nil {
		dns = &dnsStream{}
	}
	done := make(chan struct{}, 2)
	pump := func(dst, src net.Conn, up bool) {
		defer func() { done <- struct{}{} }()
		bp := bufPool.Get().(*[]byte)
		defer bufPool.Put(bp)
		buf := *bp
		for {
			if half.Load() {
				src.SetReadDeadline(time.Now().Add(halfCloseIdle))
			}
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
				if up {
					e.addSent(f, n)
				} else {
					e.addRecv(f, n)
					if dns != nil {
						dns.feed(buf[:n], e.hooks.Load().DNS)
					}
				}
			}
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) && half.Load() && f.idleFor() < halfCloseIdle {
					continue
				}
				if hc, ok := dst.(halfCloser); ok && !errors.Is(err, os.ErrDeadlineExceeded) {
					hc.CloseWrite()
				} else {
					dst.Close()
				}
				return
			}
		}
	}
	go pump(remote, app, true)
	go pump(app, remote, false)
	<-done
	half.Store(true)
	// Wake the other direction so it starts applying the idle limit.
	app.SetReadDeadline(time.Now())
	remote.SetReadDeadline(time.Now())
	<-done
	app.Close()
	remote.Close()
}

// relayUDP copies datagrams between the app's association and the real
// socket until neither side has sent anything for idle.
func (e *Engine) relayUDP(f *flow, app, remote net.Conn, idle time.Duration) {
	dnsHook := e.hooks.Load().DNS
	tap := f.RemotePort == 53 && dnsHook != nil
	done := make(chan struct{}, 2)
	pump := func(dst, src net.Conn, up bool) {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 64<<10)
		for {
			src.SetReadDeadline(time.Now().Add(idle))
			n, err := src.Read(buf)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) && f.idleFor() < idle {
					continue
				}
				return
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				// A refused datagram (ICMP unreachable on a connected
				// socket) isn't fatal for UDP; a closed socket ends the
				// next Read.
				continue
			}
			if up {
				e.addSent(f, n)
			} else {
				e.addRecv(f, n)
				if tap {
					dnsHook(append([]byte(nil), buf[:n]...))
				}
			}
		}
	}
	go pump(remote, app, true)
	go pump(app, remote, false)
	<-done
	app.Close()
	remote.Close()
	<-done
}

// drainUDP reads and discards the app's datagrams (a blocked or
// undeliverable flow) until it has been quiet for idle, so its later
// packets don't each open a new flow.
func (e *Engine) drainUDP(f *flow, app net.Conn, idle time.Duration) {
	buf := make([]byte, 2048)
	for {
		app.SetReadDeadline(time.Now().Add(idle))
		if _, err := app.Read(buf); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) && f.idleFor() < idle {
				continue
			}
			return
		}
		f.touch()
	}
}

// serveDNS answers the app's queries to the virtual resolver by
// forwarding each to the upstream servers.
func (e *Engine) serveDNS(f *flow, app net.Conn) {
	var wmu sync.Mutex
	e.mu.Lock()
	owner, pkg := f.App, f.Package
	e.mu.Unlock()
	buf := make([]byte, 64<<10)
	for {
		app.SetReadDeadline(time.Now().Add(dnsIdle))
		n, err := app.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) && f.idleFor() < dnsIdle {
				continue
			}
			return
		}
		if n < 12 {
			continue
		}
		e.addSent(f, n)
		if filter := e.hooks.Load().Filter; filter != nil {
			if name, ok := questionName(buf[:n]); ok && filter(owner, pkg, name) {
				if resp := refusedReply(buf[:n]); resp != nil {
					wmu.Lock()
					_, err := app.Write(resp)
					wmu.Unlock()
					if err == nil {
						e.addRecv(f, len(resp))
					}
				}
				continue
			}
		}
		select {
		case e.dnsSem <- struct{}{}:
		default:
			continue // too many queries in flight: drop, the app retries
		}
		q := append([]byte(nil), buf[:n]...)
		f.run.wg.Add(1)
		go func() {
			defer f.run.wg.Done()
			defer func() { <-e.dnsSem }()
			resp, err := e.exchange(f.run.ctx, q)
			if err != nil {
				return
			}
			if h := e.hooks.Load().DNS; h != nil {
				h(append([]byte(nil), resp...))
			}
			wmu.Lock()
			_, err = app.Write(resp)
			wmu.Unlock()
			if err == nil {
				e.addRecv(f, len(resp))
			}
		}()
	}
}

// exchange sends one DNS query to the upstream servers in turn until one
// answers with the query's ID.
func (e *Engine) exchange(ctx context.Context, q []byte) ([]byte, error) {
	servers := *e.dns.Load()
	if len(servers) == 0 {
		return nil, errors.New("vpn: no upstream DNS servers")
	}
	deadline := time.Now().Add(dnsTimeout)
	err := errors.New("vpn: DNS timeout")
	for _, s := range servers {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		per := min(left, dnsPerServer)
		dctx, cancel := context.WithTimeout(ctx, per)
		c, derr := e.dial(dctx, "udp", s)
		cancel()
		if derr != nil {
			err = derr
			continue
		}
		resp, xerr := exchangeOn(c, q, time.Now().Add(per))
		c.Close()
		if xerr == nil {
			return resp, nil
		}
		err = xerr
		if ctx.Err() != nil {
			return nil, errStopped
		}
	}
	return nil, err
}

func exchangeOn(c net.Conn, q []byte, deadline time.Time) ([]byte, error) {
	c.SetDeadline(deadline)
	if _, err := c.Write(q); err != nil {
		return nil, err
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return nil, err
		}
		if n >= 12 && buf[0] == q[0] && buf[1] == q[1] {
			return append([]byte(nil), buf[:n]...), nil
		}
	}
}

// dnsStream picks DNS messages out of a TCP stream (each preceded by its
// 16-bit length).
type dnsStream struct {
	buf []byte
	bad bool
}

func (s *dnsStream) feed(p []byte, hook func([]byte)) {
	if s.bad || hook == nil {
		return
	}
	s.buf = append(s.buf, p...)
	for len(s.buf) >= 2 {
		n := int(binary.BigEndian.Uint16(s.buf))
		if n < 12 {
			s.bad, s.buf = true, nil // not DNS after all
			return
		}
		if len(s.buf) < 2+n {
			break
		}
		hook(append([]byte(nil), s.buf[2:2+n]...))
		s.buf = s.buf[2+n:]
	}
	if len(s.buf) == 0 {
		s.buf = nil
	}
}
