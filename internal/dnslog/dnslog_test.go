package dnslog

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestLookupAndNamesOrder(t *testing.T) {
	l := New()
	base := time.Unix(1000, 0)
	l.Add(Answer{Name: "a.example.", IP: "192.0.2.1", At: base})
	l.Add(Answer{Name: "B.example", IP: "192.0.2.1", At: base.Add(time.Second)})
	l.Add(Answer{Name: "c.example", IP: "192.0.2.1", At: base.Add(2 * time.Second)})
	l.Add(Answer{Name: "other.example", IP: "192.0.2.2", At: base})
	if got := l.Names("192.0.2.1"); !slices.Equal(got, []string{"c.example", "b.example", "a.example"}) {
		t.Errorf("Names = %v", got)
	}
	// Seeing a name again makes it the newest without duplicating it.
	l.Add(Answer{Name: "a.example", IP: "192.0.2.1", At: base.Add(3 * time.Second)})
	if got := l.Names("192.0.2.1"); !slices.Equal(got, []string{"a.example", "c.example", "b.example"}) {
		t.Errorf("Names after refresh = %v", got)
	}
	if name, ok := l.Lookup("192.0.2.1"); !ok || name != "a.example" {
		t.Errorf("Lookup = %q %v", name, ok)
	}
	if _, ok := l.Lookup("192.0.2.3"); ok {
		t.Error("Lookup of an unseen address succeeded")
	}
	if _, ok := l.Lookup("not an ip"); ok {
		t.Error("Lookup of garbage succeeded")
	}
	if l.Names("192.0.2.3") != nil {
		t.Error("Names of an unseen address is not nil")
	}
	if l.Len() != 4 {
		t.Errorf("Len = %d, want 4", l.Len())
	}
}

func TestAddressForms(t *testing.T) {
	l := New()
	l.Add(Answer{Name: "v6.example", IP: "2001:DB8:0:0::1"})
	l.Add(Answer{Name: "mapped.example", IP: "::ffff:192.0.2.7"})
	l.Add(Answer{Name: "", IP: "192.0.2.8"})
	l.Add(Answer{Name: "bad.example", IP: "nope"})
	if name, ok := l.Lookup("2001:db8::1"); !ok || name != "v6.example" {
		t.Errorf("Lookup v6 = %q %v", name, ok)
	}
	if name, ok := l.Lookup("192.0.2.7"); !ok || name != "mapped.example" {
		t.Errorf("Lookup mapped = %q %v", name, ok)
	}
	if name, ok := l.Lookup("::ffff:192.0.2.7"); !ok || name != "mapped.example" {
		t.Errorf("Lookup mapped form = %q %v", name, ok)
	}
	if l.Len() != 2 {
		t.Errorf("Len = %d, want 2", l.Len())
	}
}

func TestBoundsEvictLeastRecent(t *testing.T) {
	l := newLog(100)
	for i := 0; i < 250; i++ {
		l.Add(Answer{Name: fmt.Sprintf("n%d.example", i), IP: fmt.Sprintf("10.0.%d.%d", i/200, i%200)})
		if i == 120 {
			// Refresh an old entry so it survives.
			l.Add(Answer{Name: "n160.example", IP: "10.0.0.160"})
		}
	}
	if l.Len() != 100 {
		t.Fatalf("Len = %d, want 100", l.Len())
	}
	if _, ok := l.Lookup("10.0.0.149"); ok {
		t.Error("an old entry survived")
	}
	if _, ok := l.Lookup("10.0.1.49"); !ok {
		t.Error("the newest entry was evicted")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.byPair) != 100 || len(l.byIP) != 100 {
		t.Errorf("indexes hold %d pairs and %d addresses, want 100", len(l.byPair), len(l.byIP))
	}
}

func TestEvictKeepsOtherNamesOfAddress(t *testing.T) {
	l := newLog(2)
	l.Add(Answer{Name: "old.example", IP: "192.0.2.1"})
	l.Add(Answer{Name: "new.example", IP: "192.0.2.1"})
	l.Add(Answer{Name: "x.example", IP: "192.0.2.2"})
	if got := l.Names("192.0.2.1"); !slices.Equal(got, []string{"new.example"}) {
		t.Errorf("Names = %v", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	l := newLog(500)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				ip := fmt.Sprintf("10.%d.%d.%d", g, i/250, i%250)
				l.Add(Answer{Name: fmt.Sprintf("h%d.example", i%7), IP: ip})
				l.Lookup(ip)
				l.Names(ip)
				l.Method()
			}
		}()
	}
	wg.Wait()
	if l.Len() != 500 {
		t.Errorf("Len = %d, want 500", l.Len())
	}
}

func TestMethod(t *testing.T) {
	l := New()
	if l.Method() == "" {
		t.Error("empty Method before Run")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Run(ctx); err != nil {
		t.Errorf("Run with a cancelled context: %v", err)
	}
	if l.Method() == "" {
		t.Error("empty Method after Run")
	}
	l.Add(Answer{Name: "vpn.example", IP: "192.0.2.1"})
	if m := l.Method(); m != "Captured by the VPN" && m != "Packet capture" {
		t.Logf("Method after Add = %q", m)
	}
}
