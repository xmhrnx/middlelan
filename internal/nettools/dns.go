package nettools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DNSRecord is a single DNS answer.
type DNSRecord struct {
	// Type is the record type: A, AAAA, CNAME, MX, NS, TXT or PTR.
	Type string
	// Value is the record data in presentation form.
	Value string
}

const dnsTimeout = 5 * time.Second

// newResolver returns a resolver using server ("1.1.1.1" or "1.1.1.1:53"),
// or the system resolver when server is empty.
func newResolver(server string) (*net.Resolver, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return net.DefaultResolver, nil
	}
	addr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		addr = net.JoinHostPort(cleanHost(server), "53")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("dns server %q: %w", server, err)
	}
	if host == "" {
		return nil, fmt.Errorf("dns server %q: missing host", server)
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}, nil
}

// isNotFound reports whether err is a "no such record" style DNS error.
func isNotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

// LookupDNS queries A, AAAA, CNAME, MX, NS and TXT records for name, or PTR
// records when name is an IP address. server optionally selects a DNS server
// ("1.1.1.1" or "1.1.1.1:53"). Missing record types are not errors; an error
// is returned only when nothing could be resolved.
func LookupDNS(ctx context.Context, name, server string) ([]DNSRecord, error) {
	name = cleanHost(name)
	if name == "" {
		return nil, errors.New("dns: empty name")
	}
	r, err := newResolver(server)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	if ip := net.ParseIP(name); ip != nil {
		names, err := r.LookupAddr(ctx, ip.String())
		if err != nil {
			return nil, fmt.Errorf("dns PTR %s: %w", name, err)
		}
		out := make([]DNSRecord, 0, len(names))
		for _, n := range names {
			out = append(out, DNSRecord{Type: "PTR", Value: strings.TrimSuffix(n, ".")})
		}
		return out, nil
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[string][]DNSRecord)
		errs    []error
	)
	run := func(typ string, fn func() ([]DNSRecord, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recs, err := fn()
			mu.Lock()
			defer mu.Unlock()
			if err != nil && !isNotFound(err) {
				errs = append(errs, fmt.Errorf("%s: %w", typ, err))
			}
			results[typ] = recs
		}()
	}
	ipLookup := func(network, typ string) func() ([]DNSRecord, error) {
		return func() ([]DNSRecord, error) {
			ips, err := r.LookupIP(ctx, network, name)
			var out []DNSRecord
			for _, ip := range ips {
				out = append(out, DNSRecord{Type: typ, Value: ip.String()})
			}
			return out, err
		}
	}
	run("A", ipLookup("ip4", "A"))
	run("AAAA", ipLookup("ip6", "AAAA"))
	run("CNAME", func() ([]DNSRecord, error) {
		c, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return nil, err
		}
		c = strings.TrimSuffix(c, ".")
		if c == "" || strings.EqualFold(c, strings.TrimSuffix(name, ".")) {
			return nil, nil
		}
		return []DNSRecord{{Type: "CNAME", Value: c}}, nil
	})
	run("MX", func() ([]DNSRecord, error) {
		mxs, err := r.LookupMX(ctx, name)
		var out []DNSRecord
		for _, mx := range mxs {
			out = append(out, DNSRecord{Type: "MX", Value: strconv.Itoa(int(mx.Pref)) + " " + strings.TrimSuffix(mx.Host, ".")})
		}
		return out, err
	})
	run("NS", func() ([]DNSRecord, error) {
		nss, err := r.LookupNS(ctx, name)
		var out []DNSRecord
		for _, ns := range nss {
			out = append(out, DNSRecord{Type: "NS", Value: strings.TrimSuffix(ns.Host, ".")})
		}
		return out, err
	})
	run("TXT", func() ([]DNSRecord, error) {
		txts, err := r.LookupTXT(ctx, name)
		var out []DNSRecord
		for _, t := range txts {
			out = append(out, DNSRecord{Type: "TXT", Value: t})
		}
		return out, err
	})
	wg.Wait()

	var out []DNSRecord
	for _, typ := range []string{"A", "AAAA", "CNAME", "MX", "NS", "TXT"} {
		out = append(out, results[typ]...)
	}
	if len(out) == 0 {
		if len(errs) > 0 {
			return nil, fmt.Errorf("dns %s: %w", name, errors.Join(errs...))
		}
		return nil, fmt.Errorf("dns %s: no records found", name)
	}
	return out, nil
}
