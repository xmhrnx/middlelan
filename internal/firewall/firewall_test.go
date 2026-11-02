package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBackend records what the firewall asks of it.
type fakeBackend struct {
	mu       sync.Mutex
	caps     Capabilities
	applies  [][]Rule
	clears   int
	current  []Rule
	applyErr error
	clearErr error
	off      bool
}

func newFake() *fakeBackend {
	return &fakeBackend{caps: Capabilities{PerApp: true, PerDestination: true, Backend: "fake"}}
}

func (b *fakeBackend) Name() string               { return "fake" }
func (b *fakeBackend) Capabilities() Capabilities { return b.caps }
func (b *fakeBackend) SystemFirewallOff() bool    { return b.off }

func (b *fakeBackend) Apply(rules []Rule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applies = append(b.applies, slices.Clone(rules))
	var pe *PartialError
	if b.applyErr != nil && !errors.As(b.applyErr, &pe) {
		return b.applyErr
	}
	b.current = slices.Clone(rules)
	return b.applyErr
}

func (b *fakeBackend) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clears++
	if b.clearErr != nil {
		return b.clearErr
	}
	b.current = nil
	return nil
}

func (b *fakeBackend) calls() (applies, clears int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.applies), b.clears
}

func (b *fakeBackend) last() []Rule {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.current)
}

// fakeDNS is a resolver whose answers tests can change.
type fakeDNS struct {
	mu    sync.Mutex
	hosts map[string][]string
}

func (d *fakeDNS) lookup(_ context.Context, host string) ([]netip.Addr, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ips, ok := d.hosts[host]
	if !ok {
		return nil, fmt.Errorf("no such host %s", host)
	}
	var out []netip.Addr
	for _, s := range ips {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func (d *fakeDNS) set(host string, ips ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hosts[host] = ips
}

var testDNSServers = []netip.Addr{netip.MustParseAddr("192.168.1.1"), netip.MustParseAddr("2001:db8::53")}

func open(t *testing.T, dir string, b Backend, dns *fakeDNS) *Firewall {
	t.Helper()
	f, err := OpenOptions(dir, b, Options{
		LookupHost: dns.lookup,
		DNSServers: func() []netip.Addr { return testDNSServers },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return f
}

func newDNS() *fakeDNS {
	return &fakeDNS{hosts: map[string][]string{
		"example.com": {"93.184.216.34", "2606:2800:220:1::1"},
		"localhost":   {"127.0.0.1", "::1"},
		"dns.lan":     {"192.168.1.1"},
	}}
}

func mustAdd(t *testing.T, f *Firewall, r Rule) Rule {
	t.Helper()
	r.Enabled = true
	got, err := f.Add(r)
	if err != nil {
		t.Fatalf("Add(%+v): %v", r, err)
	}
	return got
}

func TestOffByDefaultTouchesNothing(t *testing.T) {
	b := newFake()
	f := open(t, t.TempDir(), b, newDNS())
	if f.Enabled() {
		t.Fatal("firewall enabled on a fresh install")
	}
	mustAdd(t, f, Rule{Dest: "203.0.113.5"})
	mustAdd(t, f, Rule{App: "Firefox", Exe: "/usr/bin/firefox"})
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	if a, c := b.calls(); a != 0 || c != 0 {
		t.Fatalf("backend used while off: %d applies, %d clears", a, c)
	}
	if f.Blocks("Firefox", "/usr/bin/firefox", "203.0.113.5") {
		t.Fatal("Blocks true while the firewall is off")
	}
	if s := f.Status(); s.Enabled || s.Applied != 0 || s.Backend != "fake" {
		t.Fatalf("status = %+v", s)
	}
}

func TestEnableDisable(t *testing.T) {
	b := newFake()
	dir := t.TempDir()
	f := open(t, dir, b, newDNS())
	r1 := mustAdd(t, f, Rule{Dest: "203.0.113.0/24", Note: "test"})
	r2 := mustAdd(t, f, Rule{App: "Firefox", Exe: "/usr/bin/firefox", Dest: "example.com"})
	paused := mustAdd(t, f, Rule{Dest: "198.51.100.7"})
	paused.Enabled = false
	if err := f.Update(paused); err != nil {
		t.Fatal(err)
	}
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	got := b.last()
	if len(got) != 2 || got[0].ID != r1.ID || got[1].ID != r2.ID {
		t.Fatalf("applied %+v", got)
	}
	if got[0].Dest != "203.0.113.0/24" || got[1].Dest != "93.184.216.34,2606:2800:220:1::1" {
		t.Fatalf("resolved dests %q, %q", got[0].Dest, got[1].Dest)
	}
	if s := f.Status(); !s.Enabled || s.Applied != 2 || len(s.Errors) != 0 {
		t.Fatalf("status = %+v", s)
	}
	st, err := loadState(filepath.Join(dir, fileName))
	if err != nil || !st.Enabled || !st.Installed {
		t.Fatalf("saved state %+v, %v", st, err)
	}

	if err := f.Disable(); err != nil {
		t.Fatal(err)
	}
	if b.last() != nil {
		t.Fatal("rules left after Disable")
	}
	if f.Enabled() || f.Blocks("", "", "203.0.113.1") {
		t.Fatal("still enabled after Disable")
	}
	st, err = loadState(filepath.Join(dir, fileName))
	if err != nil || st.Enabled || st.Installed || len(st.Rules) != 3 {
		t.Fatalf("saved state after Disable %+v, %v", st, err)
	}
	// Disable is idempotent.
	if err := f.Disable(); err != nil {
		t.Fatal(err)
	}
}

func TestChangesApplyWhileEnabled(t *testing.T) {
	b := newFake()
	f := open(t, t.TempDir(), b, newDNS())
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	r := mustAdd(t, f, Rule{Dest: "203.0.113.5"})
	if got := b.last(); len(got) != 1 || got[0].ID != r.ID {
		t.Fatalf("after Add: %+v", got)
	}
	r.Dest = "198.51.100.0/25"
	created := r.Created
	r.Created = time.Time{}
	if err := f.Update(r); err != nil {
		t.Fatal(err)
	}
	if got := b.last(); len(got) != 1 || got[0].Dest != "198.51.100.0/25" {
		t.Fatalf("after Update: %+v", got)
	}
	if got := f.Rules()[0].Created; !got.Equal(created) {
		t.Fatalf("Update changed Created: %v != %v", got, created)
	}
	if err := f.Remove(r.ID); err != nil {
		t.Fatal(err)
	}
	if got := b.last(); len(got) != 0 {
		t.Fatalf("after Remove: %+v", got)
	}
	if err := f.Remove(r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove of unknown ID: %v", err)
	}
	if err := f.Update(Rule{ID: "nope", Dest: "203.0.113.1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update of unknown ID: %v", err)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	b := newFake()
	f := open(t, dir, b, newDNS())
	r := mustAdd(t, f, Rule{App: "curl", Exe: "/usr/bin/curl", Dest: "203.0.113.0/24", Note: "n"})
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	b2 := newFake()
	g := open(t, dir, b2, newDNS())
	if !g.Enabled() {
		t.Fatal("enabled flag not persisted")
	}
	rules := g.Rules()
	if len(rules) != 1 || rules[0].ID != r.ID || rules[0].Exe != r.Exe || rules[0].Note != "n" || !rules[0].Created.Equal(r.Created) {
		t.Fatalf("rules not persisted: %+v vs %+v", rules, r)
	}
	if a, c := b2.calls(); a != 0 || c != 0 {
		t.Fatal("Open touched the backend")
	}
	if !g.Blocks("curl", "/usr/bin/curl", "203.0.113.9") {
		t.Fatal("reopened firewall doesn't block")
	}
	if err := g.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := b2.last(); len(got) != 1 {
		t.Fatalf("Refresh applied %+v", got)
	}
}

func TestRejectsDangerousRules(t *testing.T) {
	f := open(t, t.TempDir(), newFake(), newDNS())
	cases := []struct {
		r    Rule
		want error
	}{
		{Rule{}, ErrBlocksEverything},
		{Rule{Note: "everything"}, ErrBlocksEverything},
		{Rule{Dest: "127.0.0.1"}, ErrProtected},
		{Rule{Dest: "127.5.0.0/16"}, ErrProtected},
		{Rule{Dest: "::1"}, ErrProtected},
		{Rule{Dest: "::ffff:127.0.0.1"}, ErrProtected},
		{Rule{Dest: "0.0.0.0/0"}, ErrProtected},
		{Rule{Dest: "::/0"}, ErrProtected},
		{Rule{Dest: "128.0.0.0/1"}, ErrProtected},
		{Rule{Dest: "255.255.255.255"}, ErrProtected},
		{Rule{Dest: "ff02::1:2"}, ErrProtected},
		{Rule{Dest: "192.168.1.1"}, ErrProtected},
		{Rule{Dest: "192.168.0.0/16"}, ErrProtected},
		{Rule{Dest: "2001:db8::/32"}, ErrProtected},
		{Rule{Dest: "localhost"}, ErrProtected},
		{Rule{Dest: "dns.lan"}, ErrProtected},
		{Rule{App: "x", Exe: "/usr/bin/x", Dest: "127.0.0.1"}, ErrProtected},
	}
	for _, c := range cases {
		if _, err := f.Add(c.r); !errors.Is(err, c.want) {
			t.Errorf("Add(%+v) = %v, want %v", c.r, err, c.want)
		}
	}
	for _, r := range []Rule{
		{Dest: "not a host!"},
		{Dest: "1.2.3"},
		{Dest: "10.0.0.0/33"},
		{App: "x", Exe: "relative/x"},
		{App: "x", Exe: `/usr/bin/x" dir=in`},
		{App: "a\tb", Dest: "203.0.113.1"},
		{Dest: "fe80::1%eth0"},
	} {
		if _, err := f.Add(r); err == nil {
			t.Errorf("Add(%+v) accepted", r)
		}
	}
	if len(f.Rules()) != 0 {
		t.Fatalf("rejected rules were stored: %+v", f.Rules())
	}
}

func TestCapabilitiesLimitRules(t *testing.T) {
	b := newFake()
	b.caps = Capabilities{PerDestination: true, Backend: "pf"}
	f := open(t, t.TempDir(), b, newDNS())
	if _, err := f.Add(Rule{App: "Safari", Dest: "203.0.113.1"}); err == nil || !strings.Contains(err.Error(), "individual programs") {
		t.Fatalf("per-app rule on a destination-only backend: %v", err)
	}
	if _, err := f.Add(Rule{Dest: "203.0.113.1"}); err != nil {
		t.Fatal(err)
	}
}

func TestBlocks(t *testing.T) {
	dns := newDNS()
	f := open(t, t.TempDir(), newFake(), dns)
	mustAdd(t, f, Rule{Dest: "203.0.113.7"})
	mustAdd(t, f, Rule{Dest: "198.51.100.0/24"})
	mustAdd(t, f, Rule{App: "Firefox", Exe: "/usr/lib/firefox/firefox"})
	mustAdd(t, f, Rule{App: "Telegram"})
	mustAdd(t, f, Rule{App: "curl", Exe: "/usr/bin/curl", Dest: "example.com"})
	mustAdd(t, f, Rule{Dest: "2001:db8:1::/48"})
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		app, exe, ip string
		want         bool
	}{
		{"", "", "203.0.113.7", true},
		{"any", "/bin/any", "203.0.113.8", false},
		{"", "", "198.51.100.200", true},
		{"", "", "198.51.101.1", false},
		{"", "", "::ffff:198.51.100.1", true},
		{"", "", "2001:db8:1:2::3", true},
		{"Firefox", "/usr/lib/firefox/firefox", "8.8.4.4", true},
		{"firefox", "", "8.8.4.4", true},
		{"Firefox", "/opt/other/firefox", "8.8.4.4", false},
		{"", "/usr/lib/firefox/firefox", "1.1.1.1", true},
		{"telegram", "", "1.1.1.1", true},
		{"curl", "/usr/bin/curl", "93.184.216.34", true},
		{"curl", "/usr/bin/curl", "2606:2800:220:1::1", true},
		{"curl", "/usr/bin/curl", "93.184.216.35", false},
		{"wget", "/usr/bin/wget", "93.184.216.34", false},
		// Protected addresses are never blocked, even for a blocked app.
		{"Firefox", "/usr/lib/firefox/firefox", "127.0.0.1", false},
		{"Firefox", "/usr/lib/firefox/firefox", "::1", false},
		{"Firefox", "/usr/lib/firefox/firefox", "192.168.1.1", false},
		{"Firefox", "/usr/lib/firefox/firefox", "255.255.255.255", false},
		{"", "", "not-an-ip", false},
	}
	for _, c := range cases {
		if got := f.Blocks(c.app, c.exe, c.ip); got != c.want {
			t.Errorf("Blocks(%q, %q, %q) = %v, want %v", c.app, c.exe, c.ip, got, c.want)
		}
	}
	// A paused rule stops matching.
	for _, r := range f.Rules() {
		if r.Dest == "203.0.113.7" {
			r.Enabled = false
			if err := f.Update(r); err != nil {
				t.Fatal(err)
			}
		}
	}
	if f.Blocks("", "", "203.0.113.7") {
		t.Fatal("paused rule still blocks")
	}
}

func TestHostNamesReResolvedOnApply(t *testing.T) {
	dns := newDNS()
	b := newFake()
	f := open(t, t.TempDir(), b, dns)
	mustAdd(t, f, Rule{Dest: "example.com"})
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	dns.set("example.com", "203.0.113.50")
	if !f.Blocks("", "", "93.184.216.34") {
		t.Fatal("old address should match until the next apply")
	}
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := b.last(); len(got) != 1 || got[0].Dest != "203.0.113.50" {
		t.Fatalf("after re-resolution: %+v", got)
	}
	if !f.Blocks("", "", "203.0.113.50") || f.Blocks("", "", "93.184.216.34") {
		t.Fatal("Blocks didn't follow the new resolution")
	}

	// Lookup failures and protected answers are reported, not silent.
	dns.set("example.com")
	dns.set("mixed.test", "203.0.113.9")
	mustAdd(t, f, Rule{Dest: "mixed.test"})
	mustAdd(t, f, Rule{Dest: "offline.test"})
	dns.set("mixed.test", "203.0.113.9", "192.168.1.1")
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	got := b.last()
	if len(got) != 1 || got[0].Dest != "203.0.113.9" {
		t.Fatalf("applied %+v", got)
	}
	s := f.Status()
	joined := strings.Join(s.Errors, "\n")
	if len(s.Errors) != 3 || !strings.Contains(joined, "offline.test") || !strings.Contains(joined, "192.168.1.1") || !strings.Contains(joined, "example.com") {
		t.Fatalf("status errors = %q", s.Errors)
	}
	if s.Applied != 1 {
		t.Fatalf("applied = %d", s.Applied)
	}
}

func TestDNSServerAddedLaterIsNotBlocked(t *testing.T) {
	b := newFake()
	servers := []netip.Addr{netip.MustParseAddr("192.168.1.1")}
	var mu sync.Mutex
	f, err := OpenOptions(t.TempDir(), b, Options{
		LookupHost: newDNS().lookup,
		DNSServers: func() []netip.Addr { mu.Lock(); defer mu.Unlock(); return slices.Clone(servers) },
	})
	if err != nil {
		t.Fatal(err)
	}
	mustAdd(t, f, Rule{Dest: "9.9.9.9"})
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	servers = append(servers, netip.MustParseAddr("9.9.9.9"))
	mu.Unlock()
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := b.last(); len(got) != 0 {
		t.Fatalf("rule against the DNS server applied: %+v", got)
	}
	if s := f.Status(); len(s.Errors) != 1 || !strings.Contains(s.Errors[0], "DNS server 9.9.9.9") {
		t.Fatalf("errors = %q", s.Errors)
	}
	if f.Blocks("", "", "9.9.9.9") {
		t.Fatal("Blocks true for a DNS server")
	}
}

func TestEnableFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	b := newFake()
	b.applyErr = errors.New("needs admin")
	f := open(t, dir, b, newDNS())
	mustAdd(t, f, Rule{Dest: "203.0.113.1"})
	if err := f.Enable(); err == nil {
		t.Fatal("Enable succeeded")
	}
	if f.Enabled() {
		t.Fatal("enabled after a failed Enable")
	}
	if _, c := b.calls(); c != 1 {
		t.Fatalf("partly applied rules not cleared: %d clears", c)
	}
	st, err := loadState(filepath.Join(dir, fileName))
	if err != nil || st.Enabled || st.Installed {
		t.Fatalf("saved %+v, %v", st, err)
	}
	if s := f.Status(); len(s.Errors) == 0 {
		t.Fatal("failure not in status")
	}
}

func TestFailedChangeIsRolledBack(t *testing.T) {
	b := newFake()
	f := open(t, t.TempDir(), b, newDNS())
	keep := mustAdd(t, f, Rule{Dest: "203.0.113.1"})
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.applyErr = errors.New("boom")
	b.mu.Unlock()
	if _, err := f.Add(Rule{Dest: "203.0.113.2", Enabled: true}); err == nil {
		t.Fatal("Add succeeded")
	}
	if err := f.Remove(keep.ID); err == nil {
		t.Fatal("Remove succeeded")
	}
	if rules := f.Rules(); len(rules) != 1 || rules[0].ID != keep.ID {
		t.Fatalf("rules after failed changes: %+v", rules)
	}
}

func TestPartialApply(t *testing.T) {
	b := newFake()
	f := open(t, t.TempDir(), b, newDNS())
	r1 := mustAdd(t, f, Rule{App: "x", Exe: "/usr/bin/x"})
	mustAdd(t, f, Rule{Dest: "203.0.113.1"})
	b.applyErr = &PartialError{Problems: []Problem{{r1.ID, "x isn't running"}}}
	if err := f.Enable(); err != nil {
		t.Fatalf("partial apply failed Enable: %v", err)
	}
	s := f.Status()
	if !s.Enabled || s.Applied != 1 || len(s.Errors) != 1 || !strings.Contains(s.Errors[0], "isn't running") {
		t.Fatalf("status = %+v", s)
	}
}

func TestCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	// A crash between turning off and removing the rules.
	write(t, dir, state{Enabled: false, Installed: true, Rules: []Rule{{ID: "abc", Dest: "203.0.113.1", Enabled: true}}})
	b := newFake()
	f := open(t, dir, b, newDNS())
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, c := b.calls(); c != 1 {
		t.Fatalf("leftovers not cleared: %d clears", c)
	}
	st, err := loadState(filepath.Join(dir, fileName))
	if err != nil || st.Installed {
		t.Fatalf("saved %+v, %v", st, err)
	}
	// A failed clear is retried on the next refresh.
	write(t, dir, state{Installed: true})
	b = newFake()
	b.clearErr = errors.New("busy")
	f = open(t, dir, b, newDNS())
	if err := f.Refresh(); err == nil {
		t.Fatal("failed clear not reported")
	}
	b.clearErr = nil
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := f.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, c := b.calls(); c != 2 {
		t.Fatalf("clears = %d, want 2", c)
	}
}

func TestOpenDropsDangerousRulesFromFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, state{Rules: []Rule{
		{ID: "everything", Enabled: true},
		{ID: "loop", Dest: "127.0.0.1", Enabled: true},
		{ID: "ok", Dest: "203.0.113.1", Enabled: true},
		{ID: "bad id\"", Dest: "203.0.113.2", Enabled: true},
	}})
	f, err := OpenOptions(dir, newFake(), Options{LookupHost: newDNS().lookup, DNSServers: func() []netip.Addr { return nil }})
	if err == nil {
		t.Fatal("dangerous rules not reported")
	}
	rules := f.Rules()
	if len(rules) != 2 || rules[0].ID != "ok" || !validID(rules[1].ID) || rules[1].Dest != "203.0.113.2" {
		t.Fatalf("rules = %+v", rules)
	}
}

func TestUninstall(t *testing.T) {
	dir := t.TempDir()
	b := newFake()
	f := open(t, dir, b, newDNS())
	mustAdd(t, f, Rule{Dest: "203.0.113.1"})
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := f.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if b.last() != nil {
		t.Fatal("rules left")
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(err) {
		t.Fatalf("rules file kept: %v", err)
	}
	if f.Enabled() || len(f.Rules()) != 0 {
		t.Fatal("state kept")
	}
}

func TestSystemFirewallOffInStatus(t *testing.T) {
	b := newFake()
	b.off = true
	f := open(t, t.TempDir(), b, newDNS())
	if !f.Status().SystemFirewallOff {
		t.Fatal("SystemFirewallOff not reported")
	}
}

func TestConcurrentUse(t *testing.T) {
	f := open(t, t.TempDir(), newFake(), newDNS())
	if err := f.Enable(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			r, err := f.Add(Rule{Dest: fmt.Sprintf("203.0.113.%d", i+1), Enabled: true})
			if err != nil {
				t.Error(err)
				return
			}
			f.Blocks("", "", "203.0.113.1")
			_ = f.Status()
			if err := f.Remove(r.ID); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			for range 50 {
				f.Blocks("app", "/bin/app", "203.0.113.3")
				f.Rules()
			}
		})
	}
	wg.Wait()
	if len(f.Rules()) != 0 {
		t.Fatalf("rules left: %+v", f.Rules())
	}
}

func write(t *testing.T, dir string, st state) {
	t.Helper()
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), b, 0o600); err != nil {
		t.Fatal(err)
	}
}
