package threat

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fixtures maps each default source's cache file to its testdata snippet.
var fixtures = map[string]string{
	"feodo.txt":            "feodo.txt",
	"urlhaus.txt":          "urlhaus_hostfile.txt",
	"spamhaus_drop.txt":    "spamhaus_drop.txt",
	"spamhaus_drop_v6.txt": "spamhaus_drop_v6.txt",
	"firehol_level1.txt":   "firehol_level1.netset",
}

// testSources are the default sources without minimum entry counts, which
// the small fixtures could not meet.
func testSources() []*source {
	s := defaultSources()
	for _, src := range s {
		src.MinEntries = 0
	}
	return s
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtureDB opens a DB whose cache holds the fixtures.
func fixtureDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "threat"), 0o700); err != nil {
		t.Fatal(err)
	}
	for file, fx := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, "threat", file), readFixture(t, fx), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d := open(dir, testSources())
	for _, li := range d.Lists() {
		if li.Err != "" || li.Updated.IsZero() {
			t.Fatalf("%s: %+v", li.Name, li)
		}
	}
	return d
}

func srcByFile(file string) *source {
	for _, s := range testSources() {
		if s.File == file {
			return s
		}
	}
	panic(file)
}

func TestParseFireHOL(t *testing.T) {
	l, err := parseList(srcByFile("firehol_level1.txt"), readFixture(t, "firehol_level1.netset"))
	if err != nil {
		t.Fatal(err)
	}
	// 6 public entries; 5.42.92.0/24 and 5.42.93.0/24 merge into one range.
	if l.entries != 6 || len(l.v4) != 5 || len(l.v6) != 0 || l.domains != nil {
		t.Fatalf("entries %d, v4 %d, v6 %d", l.entries, len(l.v4), len(l.v6))
	}
	for i := 1; i < len(l.v4); i++ {
		if l.v4[i].lo <= l.v4[i-1].hi {
			t.Fatalf("ranges not sorted/merged: %+v", l.v4)
		}
	}
}

func TestParseSpamhaus(t *testing.T) {
	l, err := parseList(srcByFile("spamhaus_drop.txt"), readFixture(t, "spamhaus_drop.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// Adjacent ranges with different SBL ids stay apart to keep the ids.
	if l.entries != 5 || len(l.v4) != 5 || len(l.details) != 5 {
		t.Fatalf("entries %d, v4 %d, details %v", l.entries, len(l.v4), l.details)
	}
	l6, err := parseList(srcByFile("spamhaus_drop_v6.txt"), readFixture(t, "spamhaus_drop_v6.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// CRLF file; 2001:db8::/32 and fc00::/7 are skipped.
	if l6.entries != 3 || len(l6.v6) != 3 {
		t.Fatalf("v6 entries %d, ranges %d", l6.entries, len(l6.v6))
	}
}

func TestParseFeodo(t *testing.T) {
	l, err := parseList(srcByFile("feodo.txt"), readFixture(t, "feodo.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if l.entries != 2 || len(l.v4) != 2 { // 192.168.1.10 skipped
		t.Fatalf("entries %d, v4 %d", l.entries, len(l.v4))
	}
}

func TestParseHosts(t *testing.T) {
	l, err := parseList(srcByFile("urlhaus.txt"), readFixture(t, "urlhaus_hostfile.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"evil-malware.example.com", "bad-host.net", "payload.cdn-evil.org", "malware.co.uk"}
	if len(l.domains) != len(want) || l.entries != len(want)+1 || len(l.v4) != 1 {
		t.Fatalf("domains %v, entries %d, v4 %d", l.domains, l.entries, len(l.v4))
	}
	for _, w := range want {
		if _, ok := l.domains[w]; !ok {
			t.Errorf("missing %s", w)
		}
	}
	// Single labels are never entries.
	l, err = parseList(srcByFile("urlhaus.txt"), []byte("127.0.0.1 com\n0.0.0.0 evil.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := l.domains["com"]; ok || len(l.domains) != 1 {
		t.Fatalf("%v", l.domains)
	}
}

func TestParseRejects(t *testing.T) {
	src := srcByFile("firehol_level1.txt")
	cases := map[string]string{
		"html":    "\xef\xbb\xbf  <!DOCTYPE html><html><body>Rate limited</body></html>",
		"garbage": "1.2.3.4\nthis is not\na blocklist\nat all\n",
	}
	for name, body := range cases {
		if _, err := parseList(src, []byte(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	strict := &source{Name: "x", MinEntries: 3}
	if _, err := parseList(strict, []byte("# only comments\n; and more\n8.8.8.8\n")); err == nil {
		t.Error("too few entries accepted")
	}
	// Mixed comment styles, inline comments, CRLF and blank lines are fine.
	l, err := parseList(src, []byte("# c\r\n; c\r\n\r\n  8.8.8.0/24  # inline\r\n9.9.9.9 ; ref\r\n"))
	if err != nil || l.entries != 2 {
		t.Fatalf("%v %+v", err, l)
	}
}

func TestPrivateRangesNeverMatch(t *testing.T) {
	d := fixtureDB(t)
	for _, ip := range []string{
		"0.0.0.0", "0.1.2.3", "10.1.2.3", "100.64.0.1", "100.127.255.255", "127.0.0.1",
		"169.254.1.1", "172.16.0.1", "172.31.255.255", "192.168.1.1", "192.168.1.10",
		"192.0.2.1", "198.18.0.1", "203.0.112.5", "203.0.113.5", "224.0.0.251",
		"239.255.255.250", "240.0.0.1", "255.255.255.255", "::1", "fe80::1%eth0", "fd00::1", "ff02::1",
	} {
		if m, ok := d.CheckIP(ip); ok {
			t.Errorf("%s flagged: %+v", ip, m)
		}
	}
	// Even a list claiming the whole Internet flags nothing local.
	l, err := parseList(&source{Name: "all"}, []byte("0.0.0.0/0\n::/0\n64.0.0.0/2\n1.0.0.0/8\n"))
	if err != nil {
		t.Fatal(err)
	}
	if l.entries != 1 { // only 1.0.0.0/8 is clear of reserved space
		t.Fatalf("entries %d", l.entries)
	}
}

func TestCheckIPEdges(t *testing.T) {
	d := fixtureDB(t)
	cases := []struct {
		ip, list, detail string
		hit              bool
	}{
		{"1.10.15.255", "", "", false},
		{"1.10.16.0", "Spamhaus DROP", "SBL256894", true},
		{"1.10.31.255", "Spamhaus DROP", "SBL256894", true},
		{"1.10.32.0", "", "", false},
		{"2.56.195.255", "Spamhaus DROP", "SBL459831", true},
		{"2.56.196.0", "Spamhaus DROP", "SBL459832", true},
		{"5.42.91.255", "", "", false},
		{"5.42.92.0", "FireHOL level1", "5.42.92.0/23", true},
		{"5.42.93.255", "FireHOL level1", "5.42.92.0/23", true},
		{"5.42.94.0", "", "", false},
		{"44.0.0.0", "FireHOL level1", "44.0.0.0/7", true}, // 45.0.0.0/7 masked
		{"45.255.255.255", "FireHOL level1", "44.0.0.0/7", true},
		{"46.0.0.0", "", "", false},
		{"50.16.16.211", "abuse.ch Feodo Tracker", "", true},
		{" 162.243.103.246 ", "abuse.ch Feodo Tracker", "", true},
		{"::ffff:50.16.16.211", "abuse.ch Feodo Tracker", "", true},
		{"50.16.16.212", "", "", false},
		{"45.95.147.10", "abuse.ch URLhaus", "", true}, // URLhaus lists it before FireHOL
		{"not an ip", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		m, ok := d.CheckIP(c.ip)
		if ok != c.hit || m.List != c.list || m.Detail != c.detail {
			t.Errorf("%q: got %v %+v", c.ip, ok, m)
		}
	}
	if m, _ := d.CheckIP("50.16.16.211"); m.Category != "botnet C2" || m.Indicator != "50.16.16.211" {
		t.Errorf("%+v", m)
	}
}

func TestCheckIPv6(t *testing.T) {
	d := fixtureDB(t)
	cases := map[string]string{
		"2a06:5280::":  "SBL474196",
		"2a06:5280::1": "SBL474196",
		"2A06:5287:ffff:ffff:ffff:ffff:ffff:ffff": "SBL474196",
		"2a06:5288::":     "",
		"2a07:9944:1::5":  "SBL593430",
		"2a07:9948::":     "",
		"2a0d:a3c0::1%3":  "SBL610000",
		"2001:db8::1":     "",
		"fc00::1":         "",
		"2606:4700::1111": "",
	}
	for ip, want := range cases {
		m, ok := d.CheckIP(ip)
		if ok != (want != "") || m.Detail != want {
			t.Errorf("%s: got %v %+v", ip, ok, m)
		}
		if ok && (m.List != "Spamhaus DROPv6" || m.Category != "spam/abuse") {
			t.Errorf("%s: %+v", ip, m)
		}
	}
}

func TestCheckDomain(t *testing.T) {
	d := fixtureDB(t)
	cases := []struct{ name, detail string }{
		{"evil-malware.example.com", ""},
		{"a.b.evil-malware.example.com", "listed: evil-malware.example.com"},
		{"EVIL-MALWARE.example.com.", ""},
		{"bad-host.net", ""},
		{"www.Bad-Host.NET", "listed: bad-host.net"},
		{"payload.cdn-evil.org", ""},
		{"x.malware.co.uk", "listed: malware.co.uk"},
	}
	for _, c := range cases {
		m, ok := d.CheckDomain(c.name)
		if !ok || m.Detail != c.detail || m.List != "abuse.ch URLhaus" || m.Category != "malware" {
			t.Errorf("%q: got %v %+v", c.name, ok, m)
		}
	}
	for _, n := range []string{
		"example.com", "com", "com.", "net", "org", "uk", "co.uk", "localhost", "",
		"cdn-evil.org", "notevil-malware.example.com", "evil-malware.example.com.evil",
	} {
		if m, ok := d.CheckDomain(n); ok {
			t.Errorf("%q flagged: %+v", n, m)
		}
	}
	// IP literals are checked as addresses.
	if m, ok := d.CheckDomain("50.16.16.211"); !ok || m.Category != "botnet C2" {
		t.Errorf("ip literal: %v %+v", ok, m)
	}
}

func TestOpenEmptyAndCorrupt(t *testing.T) {
	d := Open("")
	if _, ok := d.CheckIP("50.16.16.211"); ok || !d.Updated().IsZero() || len(d.Lists()) != 5 {
		t.Fatalf("empty db: %+v", d.Lists())
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "threat"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "threat", "feodo.txt"), []byte("<html>oops</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	d = open(dir, testSources())
	if li := d.Lists()[0]; li.Entries != 0 || !strings.Contains(li.Err, "cached copy unusable") {
		t.Fatalf("%+v", li)
	}
}

// listServer serves the fixtures over HTTPS with ETags and counts requests.
type listServer struct {
	*httptest.Server
	mu       sync.Mutex
	body     map[string][]byte // path -> body; missing path -> 500
	requests atomic.Int64
	notMod   atomic.Int64
}

func newListServer(t *testing.T) *listServer {
	s := &listServer{body: map[string][]byte{}}
	for file, fx := range fixtures {
		s.body["/"+file] = readFixture(t, fx)
	}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		if r.UserAgent() != "MiddleLAN" {
			http.Error(w, "bad agent", http.StatusForbidden)
			return
		}
		s.mu.Lock()
		b, ok := s.body[r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "broken", http.StatusInternalServerError)
			return
		}
		etag := fmt.Sprintf(`"%x"`, sha256.Sum256(b))
		if r.Header.Get("If-None-Match") == etag {
			s.notMod.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write(b)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *listServer) set(path string, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b == nil {
		delete(s.body, path)
	} else {
		s.body[path] = b
	}
}

// serverDB opens an empty DB in a temp dir whose lists come from s.
func serverDB(t *testing.T, s *listServer, dir string) *DB {
	srcs := testSources()
	for _, src := range srcs {
		src.URL = s.URL + "/" + src.File
	}
	d := open(dir, srcs)
	d.client = s.Client()
	d.client.CheckRedirect = httpsOnlyRedirect
	return d
}

func TestUpdate(t *testing.T) {
	s := newListServer(t)
	dir := t.TempDir()
	d := serverDB(t, s, dir)
	if err := d.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.CheckIP("50.16.16.211"); !ok || d.Updated().IsZero() {
		t.Fatal("update did not load lists")
	}
	// Saved atomically: files and state are there, no temp files left.
	ents, err := os.ReadDir(filepath.Join(dir, "threat"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != len(fixtures)+1 {
		t.Fatalf("cache dir: %v", ents)
	}

	// Unchanged lists answer 304.
	if err := d.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := s.notMod.Load(); n != int64(len(fixtures)) {
		t.Fatalf("304s: %d", n)
	}

	// Failing and bogus lists keep their old copy and report why.
	s.set("/feodo.txt", nil)
	s.set("/spamhaus_drop.txt", []byte("<html><body>Access denied</body></html>"))
	err = d.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Feodo") || !strings.Contains(err.Error(), "Spamhaus DROP") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := d.CheckIP("50.16.16.211"); !ok {
		t.Fatal("failed list lost its old copy")
	}
	if m, ok := d.CheckIP("1.10.16.1"); !ok || m.List != "Spamhaus DROP" {
		t.Fatalf("rejected list lost its old copy: %+v", m)
	}
	li := d.Lists()
	if !strings.Contains(li[0].Err, "500") || !strings.Contains(li[2].Err, "HTML") || li[0].Entries != 2 || li[4].Err != "" {
		t.Fatalf("%+v", li)
	}

	// A new copy replaces the old one and clears the error.
	s.set("/feodo.txt", []byte("# new\n8.8.4.4\n"))
	_ = d.Update(context.Background())
	if _, ok := d.CheckIP("50.16.16.211"); ok {
		t.Fatal("old entry still listed")
	}
	if m, ok := d.CheckIP("8.8.4.4"); !ok || m.List != "abuse.ch Feodo Tracker" || d.Lists()[0].Err != "" {
		t.Fatalf("new entry: %+v %+v", m, d.Lists()[0])
	}

	// A restart loads what was saved, with validators for the next update.
	d2 := serverDB(t, s, dir)
	if _, ok := d2.CheckIP("8.8.4.4"); !ok {
		t.Fatal("cache not reloaded")
	}
	if _, ok := d2.CheckDomain("x.bad-host.net"); !ok {
		t.Fatal("domains not reloaded")
	}
	before := s.notMod.Load()
	_ = d2.Update(context.Background())
	if s.notMod.Load()-before < 3 {
		t.Fatal("validators not reused after restart")
	}
}

func TestUpdateOversizedAndHTTP(t *testing.T) {
	s := newListServer(t)
	big := strings.Repeat("8.8.8.8\n", maxDownload/8+1)
	s.set("/firehol_level1.txt", []byte(big))
	d := serverDB(t, s, "")
	d.sources[0].URL = strings.Replace(d.sources[0].URL, "https://", "http://", 1)
	err := d.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "larger than") || !strings.Contains(err.Error(), "https") {
		t.Fatalf("err = %v", err)
	}
	if d.Lists()[4].Entries != 0 || d.Lists()[0].Entries != 0 {
		t.Fatalf("%+v", d.Lists())
	}
}

func TestRedirectToHTTPRefused(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "8.8.8.8\n")
	}))
	defer plain.Close()
	redir := httptest.NewTLSServer(http.RedirectHandler(plain.URL+"/x", http.StatusFound))
	defer redir.Close()
	d := open("", testSources())
	d.client = redir.Client()
	d.client.CheckRedirect = httpsOnlyRedirect
	d.sources = d.sources[:1]
	d.sources[0].URL = redir.URL + "/feodo.txt"
	d.snap.Store(&snapshot{lists: d.snap.Load().lists[:1]})
	err := d.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckDuringUpdate(t *testing.T) {
	s := newListServer(t)
	d := serverDB(t, s, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for ctx.Err() == nil {
				d.CheckIP("50.16.16.211")
				d.CheckIP("2a06:5280::1")
				d.CheckDomain("a.evil-malware.example.com")
				_ = d.Lists()
				_ = d.Updated()
			}
		})
	}
	for i := range 6 {
		if i%2 == 1 {
			s.set("/feodo.txt", []byte("8.8.4.4\n"))
		} else {
			s.set("/feodo.txt", readFixture(t, "feodo.txt"))
		}
		if err := d.Update(context.Background()); err != nil {
			t.Error(err)
		}
	}
	cancel()
	wg.Wait()
}

func TestRun(t *testing.T) {
	s := newListServer(t)
	d := serverDB(t, s, t.TempDir())
	d.startDelay = time.Millisecond

	// Disabled: nothing is downloaded.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	d.Run(ctx, time.Hour, func() bool { return false })
	cancel()
	if s.requests.Load() != 0 {
		t.Fatalf("downloaded while disabled: %d", s.requests.Load())
	}

	ctx, cancel = context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.Run(ctx, time.Hour, nil); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	meta := filepath.Join(d.dir, "threat", metaFile)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(meta); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if _, ok := d.CheckIP("50.16.16.211"); !ok {
		t.Fatal("Run did not update")
	}

	// Fresh lists are not downloaded again on restart.
	n := s.requests.Load()
	d2 := serverDB(t, s, d.dir)
	d2.startDelay = time.Millisecond
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	d2.Run(ctx, time.Hour, nil)
	cancel()
	if s.requests.Load() != n {
		t.Fatal("fresh cache downloaded again")
	}
}
