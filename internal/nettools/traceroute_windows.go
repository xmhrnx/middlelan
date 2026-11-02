//go:build windows

package nettools

import (
	"context"
	"net"
	"time"
)

// platformHopProber returns an IcmpSendEcho based prober for IPv4 targets,
// or nil to use the system ping fallback.
func platformHopProber(ip net.IP) hopProber {
	if ip.To4() == nil {
		return nil
	}
	if err := procIcmpSendEcho.Find(); err != nil {
		return nil
	}
	return func(ctx context.Context, ttl int) (Hop, error) {
		hop := Hop{TTL: ttl}
		if err := ctx.Err(); err != nil {
			return hop, err
		}
		h, err := icmpOpen()
		if err != nil {
			return hop, err
		}
		res, err := h.echo(ip, ttl, uint32(hopTimeout/time.Millisecond))
		if cerr := h.close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return hop, err
		}
		switch {
		case res.Status == ipSuccess:
			hop.Reached = true
			hop.Addr = ip.String()
			if res.From != nil {
				hop.Addr = res.From.String()
			}
			hop.RTT = time.Duration(res.RTTms) * time.Millisecond
		case res.From != nil && res.Status != ipReqTimedOut:
			hop.Addr = res.From.String()
			hop.RTT = time.Duration(res.RTTms) * time.Millisecond
		default:
			hop.Timeout = true
		}
		return hop, nil
	}
}
