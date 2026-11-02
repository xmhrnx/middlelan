package nettools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

var ssdpAddr = &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}

// ssdpResponse is one parsed M-SEARCH reply.
type ssdpResponse struct {
	IP, Location, Server, ST, USN string
}

// ssdpSearch multicasts an M-SEARCH for st and collects the replies until
// timeout expires.
func ssdpSearch(ctx context.Context, st string, timeout time.Duration) ([]ssdpResponse, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("ssdp listen: %w", err)
	}
	defer conn.Close() //nolint:errcheck // UDP close cannot fail meaningfully
	mx := int(timeout / time.Second)
	if mx < 1 {
		mx = 1
	}
	if mx > 5 {
		mx = 5
	}
	req := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: %d\r\nST: %s\r\nUSER-AGENT: MiddleLAN/1.0 UPnP/1.1\r\n\r\n", mx, st)
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	// Send twice: UDP multicast is lossy.
	for i := 0; i < 2; i++ {
		if _, err := conn.WriteToUDP([]byte(req), ssdpAddr); err != nil {
			return nil, fmt.Errorf("ssdp send: %w", err)
		}
	}
	var out []ssdpResponse
	buf := make([]byte, 4096)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return out, ctx.Err()
			}
			return out, fmt.Errorf("ssdp read: %w", err)
		}
		if r, ok := parseSSDPResponse(buf[:n]); ok {
			r.IP = src.IP.String()
			out = append(out, r)
		}
	}
}

// parseSSDPResponse parses the HTTP-over-UDP reply to an M-SEARCH.
func parseSSDPResponse(b []byte) (ssdpResponse, bool) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(b)), nil)
	if err != nil {
		return ssdpResponse{}, false
	}
	_ = resp.Body.Close() // in-memory body
	if resp.StatusCode != http.StatusOK {
		return ssdpResponse{}, false
	}
	h := resp.Header
	return ssdpResponse{
		Location: strings.TrimSpace(h.Get("Location")),
		Server:   strings.TrimSpace(h.Get("Server")),
		ST:       strings.TrimSpace(h.Get("St")),
		USN:      strings.TrimSpace(h.Get("Usn")),
	}, true
}
