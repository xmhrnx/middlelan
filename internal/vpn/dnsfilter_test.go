package vpn

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func query(t *testing.T, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	m := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 0x1234, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET}},
	}
	b, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQuestionName(t *testing.T) {
	name, ok := questionName(query(t, "Ads.Example.COM.", dnsmessage.TypeA))
	if !ok || name != "ads.example.com" {
		t.Fatalf("got %q %v", name, ok)
	}
	if _, ok := questionName([]byte{1, 2, 3}); ok {
		t.Error("parsed garbage")
	}
}

func TestRefusedReply(t *testing.T) {
	for _, tc := range []struct {
		typ   dnsmessage.Type
		rcode dnsmessage.RCode
		ans   int
	}{
		{dnsmessage.TypeA, dnsmessage.RCodeSuccess, 1},
		{dnsmessage.TypeAAAA, dnsmessage.RCodeSuccess, 1},
		{dnsmessage.TypeHTTPS, dnsmessage.RCodeNameError, 0},
	} {
		var m dnsmessage.Message
		if err := m.Unpack(refusedReply(query(t, "ads.example.com.", tc.typ))); err != nil {
			t.Fatal(err)
		}
		if m.ID != 0x1234 || !m.Response || m.RCode != tc.rcode || len(m.Answers) != tc.ans || len(m.Questions) != 1 {
			t.Errorf("%v: %+v", tc.typ, m)
		}
		if tc.ans == 1 {
			switch b := m.Answers[0].Body.(type) {
			case *dnsmessage.AResource:
				if b.A != [4]byte{} {
					t.Errorf("A %v", b.A)
				}
			case *dnsmessage.AAAAResource:
				if b.AAAA != [16]byte{} {
					t.Errorf("AAAA %v", b.AAAA)
				}
			}
		}
	}
}
