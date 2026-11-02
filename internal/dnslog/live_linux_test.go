//go:build linux && !android

package dnslog

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// TestLiveCapture runs a real packet capture. It needs root and is only
// run when DNSLOG_LIVE=1. It sends a crafted answer from 127.0.0.1:53 over
// the loopback interface, then, best effort, a real query to the first
// resolver in /etc/resolv.conf.
func TestLiveCapture(t *testing.T) {
	if os.Getenv("DNSLOG_LIVE") != "1" {
		t.Skip("set DNSLOG_LIVE=1 (as root) to run a live capture")
	}
	l := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	for i := 0; i < 50 && l.Method() == "Not started"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if m := l.Method(); m != "Packet capture" {
		cancel()
		t.Fatalf("Method = %q", m)
	}

	src, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53})
	if err != nil {
		t.Fatalf("binding port 53: %v", err)
	}
	defer func() {
		if err := src.Close(); err != nil {
			t.Error(err)
		}
	}()
	msg := buildResponse(t, "www.live-test.example.", dnsmessage.TypeA,
		[2]any{"www.live-test.example.", cname(t, "edge.cdn.example.")},
		[2]any{"edge.cdn.example.", a4("198.51.100.77")})
	if _, err := src.WriteToUDP(msg, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40053}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, l, "198.51.100.77", "www.live-test.example")
	if got := l.Names("198.51.100.77"); len(got) != 1 {
		t.Errorf("loopback answer recorded %d times: %v", len(got), got)
	}

	if resolver := firstResolver(); resolver != "" {
		r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", net.JoinHostPort(resolver, "53"))
		}}
		qctx, qcancel := context.WithTimeout(ctx, 5*time.Second)
		addrs, err := r.LookupHost(qctx, "example.com")
		qcancel()
		if err != nil {
			t.Logf("real query to %s failed (outbound DNS may be blocked): %v", resolver, err)
		} else {
			name, ok := l.Lookup(addrs[0])
			t.Logf("real query to %s: %v; Lookup(%s) = %q %v", resolver, addrs, addrs[0], name, ok)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Run did not return after cancel")
	}
}

func waitFor(t *testing.T, l *Log, ip, want string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if name, ok := l.Lookup(ip); ok {
			if name != want {
				t.Errorf("Lookup(%s) = %q, want %q", ip, name, want)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("no answer captured for %s", ip)
}

// firstResolver returns the first nameserver in /etc/resolv.conf.
func firstResolver() string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "nameserver" {
			return f[1]
		}
	}
	return ""
}
