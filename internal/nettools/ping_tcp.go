package nettools

import (
	"context"
	"net"
	"strconv"
	"time"
)

// tcpPinger measures TCP connect time to port 443 or 80. A refused
// connection still proves the host is reachable and counts as a reply.
type tcpPinger struct {
	ip   net.IP
	port int // 0 until a responsive port has been found
}

func newTCPPinger(ip net.IP) *tcpPinger { return &tcpPinger{ip: ip} }

func (p *tcpPinger) method() string { return "tcp" }

func (p *tcpPinger) close() error { return nil }

// tcpProbe dials ip:port and reports whether the host answered (connected or
// refused) along with the elapsed time.
func tcpProbe(ctx context.Context, ip net.IP, port int, timeout time.Duration) (bool, time.Duration, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	rtt := time.Since(start)
	if err == nil {
		if cerr := c.Close(); cerr != nil {
			return true, rtt, cerr
		}
		return true, rtt, nil
	}
	if isConnRefused(err) {
		return true, rtt, nil
	}
	return false, rtt, err
}

func (p *tcpPinger) ping(ctx context.Context, seq int, timeout time.Duration) (PingReply, error) {
	reply := PingReply{Seq: seq, Addr: p.ip.String()}
	ports := []int{443, 80}
	if p.port != 0 {
		ports = []int{p.port}
	}
	var lastErr error
	for _, port := range ports {
		ok, rtt, err := tcpProbe(ctx, p.ip, port, timeout)
		if ok {
			p.port = port
			reply.OK, reply.RTT = true, rtt
			return reply, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr != nil {
		reply.Err = lastErr.Error()
	}
	return reply, nil
}
