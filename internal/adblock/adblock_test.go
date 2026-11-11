package adblock

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const hostsList = `# StevenBlack-style hosts file
127.0.0.1 localhost
0.0.0.0 0.0.0.0
0.0.0.0 ads.example.com # inline comment
0.0.0.0 tracker.example.net metrics.example.org
::1 ip6-localhost
192.168.1.1 router.example.com
`

const filterList = `! AdGuard-style DNS filter
||doubleclick.example^
||adnetwork.example^$important
||skipped.example^$third-party
@@||good.doubleclick.example^
/regex.*/
example.com##.banner
*.wild.example^
`

func TestParse(t *testing.T) {
	r := merge([]*rules{parse([]byte(hostsList)), parse([]byte(filterList))})
	for name, want := range map[string]bool{
		"ads.example.com":              true,
		"sub.ads.example.com":          false, // hosts entries are exact
		"example.com":                  false,
		"tracker.example.net":          true,
		"metrics.example.org":          true,
		"localhost":                    false,
		"router.example.com":           false, // not a blocking address
		"doubleclick.example":          true,
		"x.y.doubleclick.example":      true, // ||name^ covers subdomains
		"good.doubleclick.example":     false,
		"cdn.good.doubleclick.example": false,
		"adnetwork.example":            true,
		"skipped.example":              false,
		"wild.example":                 false,
		"notdoubleclick.example":       false,
	} {
		if got := r.match(name); got != want {
			t.Errorf("%s: blocked %v, want %v", name, got, want)
		}
	}
}

type fakeHTTP map[string]string

func (f fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	body, ok := f[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Etag": {`"v1"`}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestBlocker(t *testing.T) {
	dir := t.TempDir()
	srcs := []Source{
		{ID: "a", Name: "A", URL: "https://lists.example/a", MinEntries: 2},
		{ID: "b", Name: "B", URL: "https://lists.example/b", MinEntries: 2},
	}
	b := &Blocker{dir: dir, sources: srcs, domains: map[string]int{}, apps: map[string]int{}}
	b.allow.Store(&map[string]bool{})
	b.lists = b.loadCache()
	b.rebuild()
	b.client = fakeHTTP{"https://lists.example/a": hostsList, "https://lists.example/b": filterList}
	if err := b.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !b.Blocked("Ads.Example.COM.") || !b.Blocked("cdn.doubleclick.example") || b.Blocked("example.com") {
		t.Fatal("matching after update")
	}
	b.SetAllowed([]string{"doubleclick.example"})
	if b.Blocked("cdn.doubleclick.example") {
		t.Error("allowed name still blocked")
	}
	for i := 0; i < 3; i++ {
		b.Record("Chrome", "ads.example.com")
	}
	b.Record("", "tracker.example.net")
	st := b.Stats()
	if st.Today != 4 || st.Total != 4 || st.TopDomains[0] != (Count{"ads.example.com", 3}) || st.TopApps[0].Name != "Chrome" || st.Recent[0].Name != "tracker.example.net" {
		t.Errorf("stats %+v", st)
	}

	// The cache is used by the next start; a failed list keeps its copy.
	b2 := &Blocker{dir: dir, sources: srcs, domains: map[string]int{}, apps: map[string]int{}}
	b2.allow.Store(&map[string]bool{})
	b2.lists = b2.loadCache()
	b2.rebuild()
	if !b2.Blocked("ads.example.com") || b2.Domains() != b.Domains() {
		t.Fatalf("cache: %d domains, want %d", b2.Domains(), b.Domains())
	}
	b2.client = fakeHTTP{}
	if err := b2.Update(context.Background()); err == nil {
		t.Error("no error for missing lists")
	}
	if !b2.Blocked("ads.example.com") || b2.Lists()[0].Err == "" {
		t.Error("failed update dropped the list or hid the error")
	}
	if b2.stale(0) {
		t.Error("retrying failed lists at once instead of in an hour")
	}
}
