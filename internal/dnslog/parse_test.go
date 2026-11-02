package dnslog

import (
	"math/rand"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func mustName(t testing.TB, s string) dnsmessage.Name {
	t.Helper()
	n, err := dnsmessage.NewName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// buildResponse builds a response to question q with the given answers,
// each one of dnsmessage.AResource, AAAAResource or CNAMEResource.
func buildResponse(t testing.TB, q string, qtype dnsmessage.Type, answers ...[2]any) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 7, Response: true, RecursionAvailable: true})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		t.Fatal(err)
	}
	if err := b.Question(dnsmessage.Question{Name: mustName(t, q), Type: qtype, Class: dnsmessage.ClassINET}); err != nil {
		t.Fatal(err)
	}
	if err := b.StartAnswers(); err != nil {
		t.Fatal(err)
	}
	for _, a := range answers {
		h := dnsmessage.ResourceHeader{Name: mustName(t, a[0].(string)), Class: dnsmessage.ClassINET, TTL: 60}
		var err error
		switch r := a[1].(type) {
		case dnsmessage.AResource:
			err = b.AResource(h, r)
		case dnsmessage.AAAAResource:
			err = b.AAAAResource(h, r)
		case dnsmessage.CNAMEResource:
			err = b.CNAMEResource(h, r)
		case dnsmessage.TXTResource:
			err = b.TXTResource(h, r)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	msg, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func a4(s string) dnsmessage.AResource {
	return dnsmessage.AResource{A: netip.MustParseAddr(s).As4()}
}

func a6(s string) dnsmessage.AAAAResource {
	return dnsmessage.AAAAResource{AAAA: netip.MustParseAddr(s).As16()}
}

func cname(t testing.TB, s string) dnsmessage.CNAMEResource {
	return dnsmessage.CNAMEResource{CNAME: mustName(t, s)}
}

func wantAnswers(t *testing.T, got []Answer, want ...string) {
	t.Helper()
	if len(got) != len(want)/2 {
		t.Fatalf("got %d answers %v, want %d", len(got), got, len(want)/2)
	}
	for i := range got {
		if got[i].Name != want[2*i] || got[i].IP != want[2*i+1] {
			t.Errorf("answer %d = %s %s, want %s %s", i, got[i].Name, got[i].IP, want[2*i], want[2*i+1])
		}
	}
}

func TestParseA(t *testing.T) {
	at := time.Unix(100, 0)
	msg := buildResponse(t, "Example.COM.", dnsmessage.TypeA,
		[2]any{"example.com.", a4("93.184.215.14")},
		[2]any{"example.com.", a4("93.184.215.15")})
	got := ParseResponse(msg, at)
	wantAnswers(t, got, "example.com", "93.184.215.14", "example.com", "93.184.215.15")
	if !got[0].At.Equal(at) {
		t.Errorf("At = %v, want %v", got[0].At, at)
	}
}

func TestParseAAAA(t *testing.T) {
	msg := buildResponse(t, "ipv6.example.", dnsmessage.TypeAAAA,
		[2]any{"ipv6.example.", a6("2001:db8::1")})
	wantAnswers(t, ParseResponse(msg, time.Now()), "ipv6.example", "2001:db8::1")
}

func TestParseCNAMEChain(t *testing.T) {
	msg := buildResponse(t, "www.youtube.com.", dnsmessage.TypeA,
		[2]any{"www.youtube.com.", cname(t, "youtube-ui.l.google.com.")},
		[2]any{"youtube-ui.l.google.com.", cname(t, "edge.l.google.com.")},
		[2]any{"edge.l.google.com.", a4("142.250.180.14")},
		[2]any{"edge.l.google.com.", dnsmessage.TXTResource{TXT: []string{"ignored"}}},
		[2]any{"unrelated.example.", a4("192.0.2.1")})
	wantAnswers(t, ParseResponse(msg, time.Now()),
		"www.youtube.com", "142.250.180.14",
		"unrelated.example", "192.0.2.1")
}

func TestParseCNAMELoop(t *testing.T) {
	msg := buildResponse(t, "a.example.", dnsmessage.TypeA,
		[2]any{"a.example.", cname(t, "b.example.")},
		[2]any{"b.example.", cname(t, "a.example.")},
		[2]any{"c.example.", a4("192.0.2.9")})
	wantAnswers(t, ParseResponse(msg, time.Now()), "c.example", "192.0.2.9")
}

func TestParseRejects(t *testing.T) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 1})
	if err := b.StartQuestions(); err != nil {
		t.Fatal(err)
	}
	if err := b.Question(dnsmessage.Question{Name: mustName(t, "q.example."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}); err != nil {
		t.Fatal(err)
	}
	query, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseResponse(query, time.Now()); got != nil {
		t.Errorf("query parsed as %v", got)
	}
	nx := buildResponse(t, "nx.example.", dnsmessage.TypeA)
	nx[3] |= 3 // RCODE NXDOMAIN
	if got := ParseResponse(nx, time.Now()); got != nil {
		t.Errorf("NXDOMAIN parsed as %v", got)
	}
	for _, m := range [][]byte{nil, {}, {0x80}, make([]byte, 12)} {
		if got := ParseResponse(m, time.Now()); got != nil {
			t.Errorf("%x parsed as %v", m, got)
		}
	}
}

func TestParseTruncatedAndGarbage(t *testing.T) {
	msg := buildResponse(t, "www.example.", dnsmessage.TypeA,
		[2]any{"www.example.", cname(t, "cdn.example.")},
		[2]any{"cdn.example.", a4("192.0.2.1")},
		[2]any{"cdn.example.", a4("192.0.2.2")})
	// Every prefix must parse without panicking; a cut in the last record
	// keeps the earlier ones.
	for n := 0; n < len(msg); n++ {
		ParseResponse(msg[:n], time.Now())
	}
	got := ParseResponse(msg[:len(msg)-1], time.Now())
	wantAnswers(t, got, "www.example", "192.0.2.1")

	rng := rand.New(rand.NewSource(1))
	buf := make([]byte, 512)
	for i := 0; i < 20000; i++ {
		n := rng.Intn(len(buf))
		rng.Read(buf[:n])
		if n > 2 && i%2 == 0 {
			buf[2] |= 0x80 // looks like a response
			if n > 3 {
				buf[3] &^= 0x0f
			}
		}
		ParseResponse(buf[:n], time.Now())
		// Also mutate a valid message.
		m := append([]byte(nil), msg...)
		m[12+rng.Intn(len(m)-12)] = byte(rng.Intn(256))
		ParseResponse(m, time.Now())
	}
}

func BenchmarkParseResponse(b *testing.B) {
	msg := buildResponse(b, "www.youtube.com.", dnsmessage.TypeA,
		[2]any{"www.youtube.com.", cname(b, "youtube-ui.l.google.com.")},
		[2]any{"youtube-ui.l.google.com.", a4("142.250.180.14")},
		[2]any{"youtube-ui.l.google.com.", a4("142.250.180.46")})
	b.ReportAllocs()
	for b.Loop() {
		ParseResponse(msg, time.Time{})
	}
}
