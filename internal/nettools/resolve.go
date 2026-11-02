package nettools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// errUnsupported is returned by platform stubs for features that are not
// available on the current operating system.
var errUnsupported = errors.New("nettools: not supported on this platform")

const (
	resolveTimeout = 5 * time.Second
	reverseTimeout = 800 * time.Millisecond
)

// cleanHost trims whitespace, URL schemes, paths and IPv6 brackets from a
// user-supplied host string.
func cleanHost(host string) string {
	h := strings.TrimSpace(host)
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if strings.HasPrefix(h, "[") {
		if i := strings.Index(h, "]"); i > 0 {
			return h[1:i]
		}
	}
	return h
}

// resolveIP resolves host to a single IP address, preferring IPv4.
func resolveIP(ctx context.Context, host string) (net.IP, error) {
	h := cleanHost(host)
	if h == "" {
		return nil, errors.New("nettools: empty host")
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip, nil
	}
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", h, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", h)
	}
	for _, a := range addrs {
		if a.IP.To4() != nil {
			return a.IP.To4(), nil
		}
	}
	return addrs[0].IP, nil
}

// reverseLookup returns the first PTR name for ip, or "" when none is found
// within a short timeout.
func reverseLookup(ctx context.Context, ip string) string {
	if ip == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, reverseTimeout)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// sleepCtx waits for d or until ctx is done, returning ctx.Err() in the
// latter case.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
