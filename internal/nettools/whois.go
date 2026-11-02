package nettools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	whoisRoot    = "whois.iana.org"
	whoisTimeout = 10 * time.Second
	whoisMaxSize = 256 << 10
)

// Whois queries whois.iana.org for a domain, IP address or AS number and
// follows the "refer:" server once, returning the referred server's answer
// (or IANA's answer when there is no referral or the referral fails).
func Whois(ctx context.Context, query string) (string, error) {
	q := cleanHost(query)
	if q == "" {
		return "", errors.New("whois: empty query")
	}
	root, err := whoisQuery(ctx, whoisRoot, q)
	if err != nil {
		return "", err
	}
	refer := whoisReferral(root)
	if refer == "" || strings.EqualFold(refer, whoisRoot) {
		return root, nil
	}
	ans, err := whoisQuery(ctx, refer, q)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return root + "\n% Referral to " + refer + " failed: " + err.Error() + "\n", nil
	}
	return ans, nil
}

// whoisReferral extracts the "refer:" (or "whois:") server from an IANA
// response.
func whoisReferral(resp string) string {
	var whois string
	sc := bufio.NewScanner(strings.NewReader(resp))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "refer":
			if v != "" {
				return v
			}
		case "whois":
			if whois == "" {
				whois = v
			}
		}
	}
	return whois
}

// whoisQuery sends q to server:43 and returns the full response.
func whoisQuery(ctx context.Context, server, q string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, whoisTimeout)
	defer cancel()
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return "", fmt.Errorf("whois %s: %w", server, err)
	}
	defer c.Close() //nolint:errcheck // read errors are reported instead
	stop := context.AfterFunc(ctx, func() { _ = c.SetDeadline(time.Now()) })
	defer stop()
	if dl, ok := ctx.Deadline(); ok {
		if err := c.SetDeadline(dl); err != nil {
			return "", err
		}
	}
	if _, err := io.WriteString(c, q+"\r\n"); err != nil {
		return "", fmt.Errorf("whois %s: %w", server, err)
	}
	b, err := io.ReadAll(io.LimitReader(c, whoisMaxSize))
	if err != nil && len(b) == 0 {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("whois %s: %w", server, err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n"), nil
}
