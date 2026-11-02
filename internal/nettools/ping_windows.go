//go:build windows

package nettools

import (
	"context"
	"fmt"
	"net"
	"time"
)

// winPinger pings through IcmpSendEcho (IPv4 only).
type winPinger struct {
	ip net.IP
	h  icmpHandle
}

// platformPingers returns the Windows-native echo technique.
func platformPingers() []pingerFactory {
	return []pingerFactory{func(ip net.IP) (pinger, error) {
		if ip.To4() == nil {
			return nil, fmt.Errorf("IcmpSendEcho: IPv6 not supported")
		}
		h, err := icmpOpen()
		if err != nil {
			return nil, err
		}
		return &winPinger{ip: ip, h: h}, nil
	}}
}

func (p *winPinger) method() string { return "system" }

func (p *winPinger) close() error { return p.h.close() }

func (p *winPinger) ping(ctx context.Context, seq int, timeout time.Duration) (PingReply, error) {
	reply := PingReply{Seq: seq, Addr: p.ip.String()}
	if d, ok := ctx.Deadline(); ok {
		if left := time.Until(d); left < timeout {
			timeout = left
		}
	}
	if timeout < 50*time.Millisecond {
		timeout = 50 * time.Millisecond
	}
	// IcmpSendEcho blocks for at most timeout (<= 2s); ctx is re-checked by
	// the caller afterwards.
	res, err := p.h.echo(p.ip, 0, uint32(timeout/time.Millisecond))
	if err != nil {
		return reply, err
	}
	switch res.Status {
	case ipSuccess:
		reply.OK = true
		reply.RTT = time.Duration(res.RTTms) * time.Millisecond
		reply.TTL = res.TTL
		if res.From != nil {
			reply.Addr = res.From.String()
		}
	case ipReqTimedOut:
		reply.Err = "request timed out"
	default:
		reply.Err = fmt.Sprintf("ICMP status %d", res.Status)
	}
	return reply, nil
}

// nativeEcho sends a single IcmpSendEcho request; used by the LAN scanner.
func nativeEcho(ip net.IP, timeout time.Duration) (bool, time.Duration, error) {
	h, err := icmpOpen()
	if err != nil {
		return false, 0, err
	}
	res, err := h.echo(ip, 0, uint32(timeout/time.Millisecond))
	if cerr := h.close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return false, 0, err
	}
	return res.Status == ipSuccess, time.Duration(res.RTTms) * time.Millisecond, nil
}
