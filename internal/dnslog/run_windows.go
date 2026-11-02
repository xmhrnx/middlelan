//go:build windows

package dnslog

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dnsapi                   = windows.NewLazySystemDLL("dnsapi.dll")
	procDnsGetCacheDataTable = dnsapi.NewProc("DnsGetCacheDataTable")
	procDnsFree              = dnsapi.NewProc("DnsFree")
)

const (
	// pollInterval is how often the DNS client cache is listed.
	pollInterval = 10 * time.Second
	// requery is how long a cached name is left alone after being read.
	requery = 2 * time.Minute
	// aliasMemory is how long a CNAME target is remembered, so it is not
	// reported as a name of its own when listed separately in the cache.
	aliasMemory = 10 * time.Minute
	// dnsQueryNoWireQuery (DNS_QUERY_NO_WIRE_QUERY) answers from the local
	// cache only, without sending anything on the network.
	dnsQueryNoWireQuery = 0x10
	// dnsFreeFlat and dnsFreeRecordList are DNS_FREE_TYPE values.
	dnsFreeFlat       = 0
	dnsFreeRecordList = 1
)

// cachePoller reads the Windows DNS client cache. No administrator rights
// are needed.
type cachePoller struct {
	log     *Log
	read    map[cacheName]time.Time // when each cached name was last read
	aliases map[string]time.Time    // CNAME targets, when last seen
}

// run polls the DNS client cache until ctx ends.
func (l *Log) run(ctx context.Context) error {
	if err := procDnsGetCacheDataTable.Find(); err != nil {
		l.setMethod("Unavailable: the DNS client cache can't be read on this Windows", false)
		return nil
	}
	if err := procDnsFree.Find(); err != nil {
		l.setMethod("Unavailable: the DNS client cache can't be read on this Windows", false)
		return nil
	}
	p := &cachePoller{log: l, read: map[cacheName]time.Time{}, aliases: map[string]time.Time{}}
	if err := p.poll(); err != nil {
		l.setMethod("Unavailable: "+err.Error(), false)
		return nil
	}
	l.setMethod("Windows DNS cache", true)
	defer l.setMethod("Windows DNS cache", false)
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		if err := p.poll(); err != nil {
			// The DNS Client service may be restarting; give up only if
			// it stays unreadable for a few minutes.
			if failures++; failures >= 30 {
				return fmt.Errorf("dnslog: reading the DNS cache: %w", err)
			}
			continue
		}
		failures = 0
	}
}

// poll lists the cache and reads the addresses of names not read recently.
func (p *cachePoller) poll() error {
	names, err := listCache()
	if err != nil {
		return err
	}
	now := time.Now()
	var direct, aliased []Answer
	listed := make(map[cacheName]bool, len(names))
	for _, n := range names {
		listed[n] = true
		if last, ok := p.read[n]; ok && now.Sub(last) < requery {
			continue
		}
		p.read[n] = now
		addrs, targets, err := queryCache(n)
		if err != nil {
			continue // expired or negative entry
		}
		for _, t := range targets {
			p.aliases[t] = now
		}
		for _, ip := range addrs {
			a := Answer{Name: n.name, IP: ip, At: now}
			if len(targets) > 0 {
				aliased = append(aliased, a)
			} else {
				direct = append(direct, a)
			}
		}
	}
	for n := range p.read {
		if !listed[n] {
			delete(p.read, n)
		}
	}
	for t, at := range p.aliases {
		if now.Sub(at) > aliasMemory {
			delete(p.aliases, t)
		}
	}
	// Names the app asked for go in last, so they win Lookup over CNAME
	// targets that are listed in the cache separately.
	for _, a := range direct {
		if _, isTarget := p.aliases[a.Name]; !isTarget {
			p.log.add(a)
		}
	}
	for _, a := range aliased {
		p.log.add(a)
	}
	return nil
}

// listCache returns the A and AAAA names in the DNS client cache and frees
// the list Windows allocated.
func listCache() ([]cacheName, error) {
	var head *cacheEntry
	r, _, callErr := procDnsGetCacheDataTable.Call(uintptr(unsafe.Pointer(&head)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_SUCCESS) || callErr == nil {
			// An empty cache also reports failure.
			return nil, nil
		}
		return nil, fmt.Errorf("listing the DNS cache: %w", callErr)
	}
	names, entries := walkCache(head)
	for _, e := range entries {
		if e.name != nil {
			dnsFree(unsafe.Pointer(e.name))
		}
		dnsFree(unsafe.Pointer(e))
	}
	return names, nil
}

// dnsFree releases memory allocated by dnsapi.dll.
func dnsFree(p unsafe.Pointer) {
	// DnsFree returns nothing; the error Call reports is meaningless.
	_, _, _ = procDnsFree.Call(uintptr(p), dnsFreeFlat)
}

// queryCache reads the cached addresses of one name without touching the
// network. It also returns the CNAME targets in the cached answer.
func queryCache(n cacheName) (addrs, targets []string, err error) {
	var rec *windows.DNSRecord
	if err := windows.DnsQuery(n.name, n.typ, dnsQueryNoWireQuery, nil, &rec, nil); err != nil {
		return nil, nil, err
	}
	defer windows.DnsRecordListFree(rec, dnsFreeRecordList)
	for r := rec; r != nil; r = r.Next {
		switch r.Type {
		case typeA, typeAAAA:
			if ip, ok := recordAddr(r.Type, r.Data[:]); ok {
				addrs = append(addrs, ip)
			}
		case typeCNAME:
			ptr := (*windows.DNSPTRData)(unsafe.Pointer(&r.Data[0]))
			if t := canonicalName(utf16PtrToString(ptr.Host)); t != "" {
				targets = append(targets, t)
			}
		}
	}
	return addrs, targets, nil
}
