package nettools

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	protoICMP   = 1
	protoICMPv6 = 58
)

// icmpPinger sends echo requests through golang.org/x/net/icmp, either over
// an unprivileged datagram socket ("udp4"/"udp6") or a raw socket.
type icmpPinger struct {
	conn       *icmp.PacketConn
	p4         *ipv4.PacketConn
	p6         *ipv6.PacketConn
	dst        net.Addr
	ip         net.IP
	v6         bool
	privileged bool
	id         int
	buf        []byte
}

// icmpNetwork returns the icmp.ListenPacket network and address for ip.
func icmpNetwork(ip net.IP, privileged bool) (network, laddr string) {
	v6 := ip.To4() == nil
	switch {
	case v6 && privileged:
		return "ip6:ipv6-icmp", "::"
	case v6:
		return "udp6", "::"
	case privileged:
		return "ip4:icmp", "0.0.0.0"
	default:
		return "udp4", "0.0.0.0"
	}
}

func newICMPPinger(ip net.IP, privileged bool) (*icmpPinger, error) {
	network, laddr := icmpNetwork(ip, privileged)
	c, err := icmp.ListenPacket(network, laddr)
	if err != nil {
		return nil, fmt.Errorf("icmp listen %s: %w", network, err)
	}
	p := &icmpPinger{
		conn:       c,
		ip:         ip,
		v6:         ip.To4() == nil,
		privileged: privileged,
		id:         rand.IntN(0xffff) + 1,
		buf:        make([]byte, 1500),
	}
	if privileged {
		p.dst = &net.IPAddr{IP: ip}
	} else {
		p.dst = &net.UDPAddr{IP: ip}
	}
	// TTL reporting is optional: when the platform cannot deliver control
	// messages the reply TTL is simply reported as 0.
	if p.v6 {
		p.p6 = c.IPv6PacketConn()
		if p.p6 != nil && p.p6.SetControlMessage(ipv6.FlagHopLimit, true) != nil {
			p.p6 = nil
		}
	} else {
		p.p4 = c.IPv4PacketConn()
		if p.p4 != nil && p.p4.SetControlMessage(ipv4.FlagTTL, true) != nil {
			p.p4 = nil
		}
	}
	return p, nil
}

func (p *icmpPinger) method() string { return "icmp" }

func (p *icmpPinger) close() error { return p.conn.Close() }

// readMsg reads one packet, returning payload length, source and TTL.
func (p *icmpPinger) readMsg() (int, net.Addr, int, error) {
	switch {
	case p.p4 != nil:
		n, cm, src, err := p.p4.ReadFrom(p.buf)
		ttl := 0
		if cm != nil {
			ttl = cm.TTL
		}
		return n, src, ttl, err
	case p.p6 != nil:
		n, cm, src, err := p.p6.ReadFrom(p.buf)
		ttl := 0
		if cm != nil {
			ttl = cm.HopLimit
		}
		return n, src, ttl, err
	default:
		n, src, err := p.conn.ReadFrom(p.buf)
		return n, src, 0, err
	}
}

func addrIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}

func (p *icmpPinger) ping(ctx context.Context, seq int, timeout time.Duration) (PingReply, error) {
	reply := PingReply{Seq: seq, Addr: p.ip.String()}
	wseq := seq & 0xffff
	var typ icmp.Type = ipv4.ICMPTypeEcho
	var replyType icmp.Type = ipv4.ICMPTypeEchoReply
	proto := protoICMP
	if p.v6 {
		typ, replyType, proto = ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply, protoICMPv6
	}
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data, uint64(time.Now().UnixNano()))
	copy(data[8:], "midlelan")
	msg := icmp.Message{Type: typ, Body: &icmp.Echo{ID: p.id, Seq: wseq, Data: data}}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return reply, err
	}

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := p.conn.SetReadDeadline(deadline); err != nil {
		return reply, err
	}
	stop := context.AfterFunc(ctx, func() {
		_ = p.conn.SetReadDeadline(time.Now()) // wake the blocked read
	})
	defer stop()

	start := time.Now()
	if _, err := p.conn.WriteTo(wb, p.dst); err != nil {
		return reply, fmt.Errorf("icmp send: %w", err)
	}
	for {
		n, src, ttl, err := p.readMsg()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				reply.Err = "request timed out"
				return reply, nil
			}
			return reply, fmt.Errorf("icmp read: %w", err)
		}
		rtt := time.Since(start)
		m, err := icmp.ParseMessage(proto, p.buf[:n])
		if err != nil {
			continue // not a parseable ICMP message; keep waiting
		}
		if m.Type != replyType {
			continue
		}
		echo, ok := m.Body.(*icmp.Echo)
		if !ok || echo.Seq != wseq {
			continue
		}
		// Unprivileged sockets have their ID rewritten by the kernel, so
		// the ID is only checked on raw sockets.
		if p.privileged && echo.ID != p.id {
			continue
		}
		if sip := addrIP(src); sip != nil && !sip.Equal(p.ip) {
			continue
		}
		reply.OK, reply.RTT, reply.TTL = true, rtt, ttl
		return reply, nil
	}
}
