package nettools

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// UPnPDevice is a device that answered an SSDP search.
type UPnPDevice struct {
	// IP is the responder's address and Location its description URL.
	IP, Location string
	// Server, ST and USN are copied from the SSDP reply.
	Server, ST, USN string
	// FriendlyName, Manufacturer and ModelName come from the device
	// description XML when it could be fetched.
	FriendlyName, Manufacturer, ModelName string
}

// DiscoverUPnP multicasts an SSDP "ssdp:all" search, waits timeout (default
// 3s) for replies and fetches each unique description document. One entry
// is returned per description URL, sorted by IP.
func DiscoverUPnP(ctx context.Context, timeout time.Duration) ([]UPnPDevice, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	resps, err := ssdpSearch(ctx, "ssdp:all", timeout)
	if err != nil {
		return nil, err
	}
	byLoc := make(map[string]*UPnPDevice)
	var order []string
	for _, r := range resps {
		key := r.Location
		if key == "" {
			key = r.IP + "|" + r.USN
		}
		if _, ok := byLoc[key]; ok {
			continue
		}
		byLoc[key] = &UPnPDevice{IP: r.IP, Location: r.Location, Server: r.Server, ST: r.ST, USN: r.USN}
		order = append(order, key)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, k := range order {
		d := byLoc[k]
		if d.Location == "" || !locationOnLAN(d.Location, d.IP) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d *UPnPDevice) {
			defer wg.Done()
			defer func() { <-sem }()
			desc, err := fetchUPnPDescription(ctx, client, d.Location)
			if err != nil {
				return // description is optional; the SSDP data remains
			}
			d.FriendlyName, d.Manufacturer, d.ModelName = desc.Device.FriendlyName, desc.Device.Manufacturer, desc.Device.ModelName
		}(d)
	}
	wg.Wait()
	out := make([]UPnPDevice, 0, len(order))
	for _, k := range order {
		out = append(out, *byLoc[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return ipLess(out[i].IP, out[j].IP) })
	return out, ctx.Err()
}

// locationOnLAN only allows description URLs that point at the responder
// itself or another private address.
func locationOnLAN(loc, responder string) bool {
	u, err := url.Parse(loc)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		return false
	}
	return ip.String() == responder || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

type upnpDescription struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		Manufacturer string `xml:"manufacturer"`
		ModelName    string `xml:"modelName"`
	} `xml:"device"`
}

func fetchUPnPDescription(ctx context.Context, client *http.Client, loc string) (upnpDescription, error) {
	var d upnpDescription
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return d, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return d, err
	}
	defer resp.Body.Close() //nolint:errcheck // body read errors are reported
	if resp.StatusCode != http.StatusOK {
		return d, fmt.Errorf("%s: HTTP %s", loc, resp.Status)
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&d); err != nil {
		return d, err
	}
	return d, nil
}

// ipLess compares two textual IP addresses numerically (IPv4 before IPv6).
func ipLess(a, b string) bool {
	ia, ib := net.ParseIP(a), net.ParseIP(b)
	if ia == nil || ib == nil {
		return a < b
	}
	if a4, b4 := ia.To4(), ib.To4(); a4 != nil && b4 != nil {
		ia, ib = a4, b4
	} else if (a4 == nil) != (b4 == nil) {
		return a4 != nil
	}
	return string(ia) < string(ib)
}

// MDNSService is a DNS-SD service instance discovered via multicast DNS.
type MDNSService struct {
	// Instance is the full instance name ("Living Room._airplay._tcp.local"),
	// Service its type ("_airplay._tcp"), Host the target host name and IP
	// its IPv4 address.
	Instance, Service, Host, IP string
	// Port is the service port from the SRV record.
	Port int
	// TXT holds the TXT record strings.
	TXT []string
}

// BrowseMDNS enumerates DNS-SD service types (_services._dns-sd._udp.local)
// and resolves each instance's SRV, TXT and A records within timeout
// (default 3s). It uses one-shot (legacy unicast) queries, which responders
// answer directly, so no port 5353 socket is needed.
func BrowseMDNS(ctx context.Context, timeout time.Duration) ([]MDNSService, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	phase := timeout / 3
	rec := newMDNSCache()

	// Phase 1: service types.
	q, err := mdnsQuestions(dnsmessage.TypePTR, "_services._dns-sd._udp.local.")
	if err != nil {
		return nil, err
	}
	if err := mdnsQuery(ctx, mdnsAddr, q, phase, rec.add); err != nil {
		return nil, err
	}
	types := rec.ptrTargets("_services._dns-sd._udp.local.")

	// Phase 2: instances of every type (records in the additional section
	// often already carry SRV/TXT/A).
	if len(types) > 0 {
		q, err := mdnsQuestions(dnsmessage.TypePTR, types...)
		if err != nil {
			return nil, err
		}
		if err := mdnsQuery(ctx, mdnsAddr, q, phase, rec.add); err != nil {
			return nil, err
		}
	}
	var instances []string
	for _, t := range types {
		instances = append(instances, rec.ptrTargets(t)...)
	}

	// Phase 3: missing SRV/TXT and addresses.
	var missing []dnsmessage.Question
	for _, in := range instances {
		if _, ok := rec.srv(in); !ok {
			qs, err := mdnsQuestions(dnsmessage.TypeSRV, in)
			if err != nil {
				continue
			}
			missing = append(missing, qs...)
			qs, err = mdnsQuestions(dnsmessage.TypeTXT, in)
			if err == nil {
				missing = append(missing, qs...)
			}
		}
	}
	if len(missing) > 0 {
		if err := mdnsQuery(ctx, mdnsAddr, capQuestions(missing), phase/2, rec.add); err != nil {
			return nil, err
		}
	}
	var hostsQ []dnsmessage.Question
	for _, in := range instances {
		if s, ok := rec.srv(in); ok && rec.addr(s.target) == "" {
			if qs, err := mdnsQuestions(dnsmessage.TypeA, s.target); err == nil {
				hostsQ = append(hostsQ, qs...)
			}
		}
	}
	if len(hostsQ) > 0 {
		if err := mdnsQuery(ctx, mdnsAddr, capQuestions(hostsQ), phase/2, rec.add); err != nil {
			return nil, err
		}
	}

	out := make([]MDNSService, 0, len(instances))
	for _, t := range types {
		for _, in := range rec.ptrTargets(t) {
			svc := MDNSService{Instance: trimDot(in), Service: strings.TrimSuffix(trimDot(t), ".local")}
			if s, ok := rec.srv(in); ok {
				svc.Host, svc.Port = trimDot(s.target), s.port
				svc.IP = rec.addr(s.target)
			}
			if svc.IP == "" {
				svc.IP = rec.srcFor(in)
			}
			svc.TXT = rec.txt(in)
			out = append(out, svc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Instance < out[j].Instance
	})
	return out, ctx.Err()
}

// capQuestions keeps a query within a single, reasonably sized datagram.
func capQuestions(q []dnsmessage.Question) []dnsmessage.Question {
	if len(q) > 24 {
		return q[:24]
	}
	return q
}

// mdnsQuestions builds questions of one type for several names.
func mdnsQuestions(t dnsmessage.Type, names ...string) ([]dnsmessage.Question, error) {
	out := make([]dnsmessage.Question, 0, len(names))
	for _, n := range names {
		if !strings.HasSuffix(n, ".") {
			n += "."
		}
		name, err := dnsmessage.NewName(n)
		if err != nil {
			return nil, fmt.Errorf("mdns name %q: %w", n, err)
		}
		out = append(out, dnsmessage.Question{Name: name, Type: t, Class: classINETUnicast})
	}
	return out, nil
}

type srvRec struct {
	target string
	port   int
}

// mdnsCache accumulates records from all responses (keys are lower-case
// names with a trailing dot).
type mdnsCache struct {
	mu    sync.Mutex
	ptrs  map[string][]string
	srvs  map[string]srvRec
	txts  map[string][]string
	addrs map[string]string
	srcs  map[string]string
}

func newMDNSCache() *mdnsCache {
	return &mdnsCache{ptrs: map[string][]string{}, srvs: map[string]srvRec{}, txts: map[string][]string{}, addrs: map[string]string{}, srcs: map[string]string{}}
}

// add records every resource; it never stops the query early.
func (c *mdnsCache) add(src net.IP, rrs []dnsmessage.Resource) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rr := range rrs {
		name := strings.ToLower(rr.Header.Name.String())
		switch b := rr.Body.(type) {
		case *dnsmessage.PTRResource:
			t := b.PTR.String()
			found := false
			for _, x := range c.ptrs[name] {
				if strings.EqualFold(x, t) {
					found = true
				}
			}
			if !found {
				c.ptrs[name] = append(c.ptrs[name], t)
			}
			c.srcs[strings.ToLower(t)] = src.String()
		case *dnsmessage.SRVResource:
			c.srvs[name] = srvRec{target: strings.ToLower(b.Target.String()), port: int(b.Port)}
		case *dnsmessage.TXTResource:
			c.txts[name] = append([]string(nil), b.TXT...)
		case *dnsmessage.AResource:
			c.addrs[name] = net.IP(b.A[:]).String()
		case *dnsmessage.AAAAResource:
			if _, ok := c.addrs[name]; !ok {
				c.addrs[name] = net.IP(b.AAAA[:]).String()
			}
		}
	}
	return false
}

func (c *mdnsCache) ptrTargets(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.ptrs[strings.ToLower(name)]...)
}

func (c *mdnsCache) srv(name string) (srvRec, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.srvs[strings.ToLower(name)]
	return s, ok
}

func (c *mdnsCache) txt(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.txts[strings.ToLower(name)]
}

func (c *mdnsCache) addr(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.addrs[strings.ToLower(name)]
}

func (c *mdnsCache) srcFor(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.srcs[strings.ToLower(name)]
}
