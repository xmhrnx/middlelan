package dnslog

import (
	"slices"
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestCacheEntryLayout(t *testing.T) {
	word := unsafe.Sizeof(uintptr(0))
	var e cacheEntry
	if got := unsafe.Offsetof(e.name); got != word {
		t.Errorf("pszName at %d, want %d", got, word)
	}
	if got := unsafe.Offsetof(e.typ); got != 2*word {
		t.Errorf("wType at %d, want %d", got, 2*word)
	}
	if got := unsafe.Offsetof(e.dataLength); got != 2*word+2 {
		t.Errorf("wDataLength at %d, want %d", got, 2*word+2)
	}
	if got := unsafe.Offsetof(e.flags); got != 2*word+4 {
		t.Errorf("dwFlags at %d, want %d", got, 2*word+4)
	}
	if got, want := unsafe.Sizeof(e), (2*word+8+word-1)&^(word-1); got != want {
		t.Errorf("size %d, want %d", got, want)
	}
}

func wide(s string) *uint16 {
	u := append(utf16.Encode([]rune(s)), 0)
	return &u[0]
}

func TestWalkCache(t *testing.T) {
	third := &cacheEntry{name: wide("v6.Example."), typ: typeAAAA}
	second := &cacheEntry{next: third, name: wide("_ldap._tcp.example"), typ: 33}
	first := &cacheEntry{next: second, name: wide("WWW.YouTube.com"), typ: typeA}
	nameless := &cacheEntry{next: first, typ: typeA}
	names, entries := walkCache(nameless)
	want := []cacheName{{"www.youtube.com", typeA}, {"v6.example", typeAAAA}}
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if len(entries) != 4 {
		t.Errorf("%d entries, want 4", len(entries))
	}
	if n, e := walkCache(nil); n != nil || e != nil {
		t.Error("walkCache(nil) returned something")
	}
}

func TestUTF16PtrToString(t *testing.T) {
	if got := utf16PtrToString(wide("bücher.example")); got != "bücher.example" {
		t.Errorf("got %q", got)
	}
	if got := utf16PtrToString(nil); got != "" {
		t.Errorf("nil gave %q", got)
	}
	long := make([]uint16, maxNameLen+10)
	for i := range long[:len(long)-1] {
		long[i] = 'a'
	}
	if got := utf16PtrToString(&long[0]); len(got) != maxNameLen {
		t.Errorf("unterminated name read %d units, want %d", len(got), maxNameLen)
	}
}

func TestRecordAddr(t *testing.T) {
	var data [40]byte
	copy(data[:], []byte{142, 250, 180, 14})
	if ip, ok := recordAddr(typeA, data[:]); !ok || ip != "142.250.180.14" {
		t.Errorf("A = %q %v", ip, ok)
	}
	copy(data[:], []byte{0x20, 0x01, 0x0d, 0xb8, 15: 1})
	if ip, ok := recordAddr(typeAAAA, data[:]); !ok || ip != "2001:db8::1" {
		t.Errorf("AAAA = %q %v", ip, ok)
	}
	if _, ok := recordAddr(typeAAAA, data[:8]); ok {
		t.Error("short AAAA accepted")
	}
	if _, ok := recordAddr(typeCNAME, data[:]); ok {
		t.Error("CNAME accepted as an address")
	}
}
