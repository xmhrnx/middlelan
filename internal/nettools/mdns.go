package nettools

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

var mdnsAddr = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// classINETUnicast is class IN with the mDNS "unicast response" bit set.
const classINETUnicast = dnsmessage.Class(0x8001)

// mdnsQuery sends questions as a one-shot ("legacy unicast") mDNS query to
// dst and passes every resource record of every response to handle until
// timeout expires, ctx is done or handle returns true.
func mdnsQuery(ctx context.Context, dst *net.UDPAddr, questions []dnsmessage.Question, timeout time.Duration, handle func(src net.IP, rrs []dnsmessage.Resource) bool) error {
	if len(questions) == 0 {
		return nil
	}
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return fmt.Errorf("mdns listen: %w", err)
	}
	defer conn.Close() //nolint:errcheck // UDP close cannot fail meaningfully
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: uint16(rand.UintN(0xffff) + 1)},
		Questions: questions,
	}
	b, err := msg.Pack()
	if err != nil {
		return fmt.Errorf("mdns pack: %w", err)
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if _, err := conn.WriteToUDP(b, dst); err != nil {
		return fmt.Errorf("mdns send: %w", err)
	}
	buf := make([]byte, 9000)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return ctx.Err()
			}
			return fmt.Errorf("mdns read: %w", err)
		}
		var m dnsmessage.Message
		if err := m.Unpack(buf[:n]); err != nil || !m.Header.Response {
			continue
		}
		rrs := make([]dnsmessage.Resource, 0, len(m.Answers)+len(m.Additionals))
		rrs = append(rrs, m.Answers...)
		rrs = append(rrs, m.Additionals...)
		if handle(src.IP, rrs) {
			return nil
		}
	}
}

// reverseName returns the in-addr.arpa name for an IPv4 address.
func reverseName(ip net.IP) string {
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", v4[3], v4[2], v4[1], v4[0])
}

// mdnsReverse resolves ip to its ".local" host name via a multicast PTR
// query. It returns "" when nothing answers within timeout.
func mdnsReverse(ctx context.Context, ip net.IP, timeout time.Duration) string {
	rn := reverseName(ip)
	if rn == "" {
		return ""
	}
	name, err := dnsmessage.NewName(rn)
	if err != nil {
		return ""
	}
	var host string
	q := []dnsmessage.Question{{Name: name, Type: dnsmessage.TypePTR, Class: classINETUnicast}}
	err = mdnsQuery(ctx, mdnsAddr, q, timeout, func(_ net.IP, rrs []dnsmessage.Resource) bool {
		for _, rr := range rrs {
			if ptr, ok := rr.Body.(*dnsmessage.PTRResource); ok && strings.EqualFold(rr.Header.Name.String(), rn) {
				host = trimDot(ptr.PTR.String())
				return true
			}
		}
		return false
	})
	if err != nil {
		return ""
	}
	return host
}

// trimDot removes a trailing dot from a DNS name.
func trimDot(s string) string { return strings.TrimSuffix(s, ".") }
