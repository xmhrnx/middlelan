package nettools

import (
	"context"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// icmpSweep pings all IPv4 hosts over a single ICMP socket and returns the
// responders with their RTT. It returns an empty map when no ICMP socket can
// be opened (the TCP probes then do the work).
func icmpSweep(ctx context.Context, hosts []net.IP, wait time.Duration) map[string]time.Duration {
	res := make(map[string]time.Duration)
	if len(hosts) == 0 {
		return res
	}
	privileged := false
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
		if err != nil {
			return res
		}
		privileged = true
	}
	defer conn.Close() //nolint:errcheck // closing ends the reader goroutine
	id := rand.IntN(0xffff) + 1

	var mu sync.Mutex
	sent := make(map[string]time.Time, len(hosts))
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 1500)
		for {
			n, src, err := conn.ReadFrom(buf)
			if err != nil {
				return // deadline reached or socket closed
			}
			now := time.Now()
			m, err := icmp.ParseMessage(protoICMP, buf[:n])
			if err != nil || m.Type != ipv4.ICMPTypeEchoReply {
				continue
			}
			echo, ok := m.Body.(*icmp.Echo)
			if !ok || (privileged && echo.ID != id) {
				continue
			}
			ip := addrIP(src)
			if ip == nil {
				continue
			}
			key := ip.String()
			mu.Lock()
			if t, ok := sent[key]; ok {
				if _, dup := res[key]; !dup {
					res[key] = now.Sub(t)
				}
			}
			mu.Unlock()
		}
	}()

	for i, ip := range hosts {
		if ctx.Err() != nil {
			break
		}
		msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: id, Seq: i & 0xffff, Data: []byte("MiddleMan")}}
		b, err := msg.Marshal(nil)
		if err != nil {
			continue
		}
		var dst net.Addr = &net.UDPAddr{IP: ip}
		if privileged {
			dst = &net.IPAddr{IP: ip}
		}
		mu.Lock()
		sent[ip.String()] = time.Now()
		mu.Unlock()
		if _, err := conn.WriteTo(b, dst); err != nil {
			continue // e.g. host unreachable: just skip it
		}
		if sleepCtx(ctx, 2*time.Millisecond) != nil {
			break
		}
	}
	end := time.Now().Add(wait)
	if err := conn.SetReadDeadline(end); err != nil {
		_ = conn.Close()
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	<-readerDone
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]time.Duration, len(res))
	for k, v := range res {
		out[k] = v
	}
	return out
}
