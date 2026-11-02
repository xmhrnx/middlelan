package dnslog

import (
	"net/netip"
	"unicode/utf16"
	"unsafe"
)

// DNS record types the Windows cache helpers care about.
const (
	typeA     = 1
	typeCNAME = 5
	typeAAAA  = 28
)

// maxCacheEntries and maxNameLen bound walks over memory handed back by
// the undocumented Windows cache API, in case it is ever malformed.
const (
	maxCacheEntries = 100000
	maxNameLen      = 1024
)

// cacheEntry mirrors DNS_CACHE_ENTRY, returned by dnsapi.dll's
// DnsGetCacheDataTable:
//
//	struct DNS_CACHE_ENTRY {
//	    DNS_CACHE_ENTRY *pNext;
//	    PWSTR pszName;
//	    unsigned short wType;
//	    unsigned short wDataLength;
//	    unsigned long dwFlags;
//	};
//
// Go lays the fields out like C does on 386, amd64 and arm64 (pointers at
// 0 and one word, then 2+2+4 bytes); TestCacheEntryLayout checks it.
type cacheEntry struct {
	next       *cacheEntry
	name       *uint16
	typ        uint16
	dataLength uint16
	flags      uint32
}

// cacheName is one name and record type listed in the DNS cache.
type cacheName struct {
	name string
	typ  uint16
}

// walkCache lists the address entries (A and AAAA) of a DNS_CACHE_ENTRY
// list, and every entry pointer so the caller can free them.
func walkCache(head *cacheEntry) (names []cacheName, entries []*cacheEntry) {
	for e := head; e != nil && len(entries) < maxCacheEntries; e = e.next {
		entries = append(entries, e)
		if e.typ != typeA && e.typ != typeAAAA {
			continue
		}
		if name := canonicalName(utf16PtrToString(e.name)); name != "" {
			names = append(names, cacheName{name, e.typ})
		}
	}
	return names, entries
}

// utf16PtrToString decodes a NUL-terminated UTF-16 string, reading at most
// maxNameLen units.
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for ptr := unsafe.Pointer(p); n < maxNameLen && *(*uint16)(ptr) != 0; n++ {
		ptr = unsafe.Add(ptr, 2)
	}
	return string(utf16.Decode(unsafe.Slice(p, n)))
}

// recordAddr decodes the address in a DNS_RECORD's data (DNS_A_DATA is
// four bytes, DNS_AAAA_DATA sixteen, both in network order).
func recordAddr(typ uint16, data []byte) (string, bool) {
	switch {
	case typ == typeA && len(data) >= 4:
		return netip.AddrFrom4([4]byte(data[:4])).String(), true
	case typ == typeAAAA && len(data) >= 16:
		return netip.AddrFrom16([16]byte(data[:16])).Unmap().String(), true
	}
	return "", false
}
