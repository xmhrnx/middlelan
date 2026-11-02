package firewall

import (
	"net/netip"
	"slices"
	"testing"
)

func TestParseDest(t *testing.T) {
	cases := []struct {
		in   string
		kind destKind
		norm string
		ok   bool
	}{
		{"", destAny, "", true},
		{" 203.0.113.5 ", destPrefix, "203.0.113.5", true},
		{"10.1.2.3/8", destPrefix, "10.0.0.0/8", true},
		{"::ffff:10.0.0.1", destPrefix, "10.0.0.1", true},
		{"::ffff:10.0.0.0/104", destPrefix, "10.0.0.0/8", true},
		{"2001:DB8::1", destPrefix, "2001:db8::1", true},
		{"Example.COM.", destHost, "example.com", true},
		{"my_host.lan", destHost, "my_host.lan", true},
		{"1.2.3", 0, "", false},
		{"-bad.com", 0, "", false},
		{"a..b", 0, "", false},
		{"10.0.0.0/40", 0, "", false},
		{"fe80::1%eth0", 0, "", false},
		{"http://x.com", 0, "", false},
	}
	for _, c := range cases {
		kind, _, norm, err := parseDest(c.in)
		if (err == nil) != c.ok || (c.ok && (kind != c.kind || norm != c.norm)) {
			t.Errorf("parseDest(%q) = %v %q %v", c.in, kind, norm, err)
		}
	}
}

func TestMinimalPrefixes(t *testing.T) {
	got := minimalPrefixes(pfx("10.1.0.0/16", "10.0.0.0/8", "10.2.3.4", "192.0.2.1", "192.0.2.1", "2001:db8::/32", "2001:db8:1::1"))
	want := pfx("10.0.0.0/8", "192.0.2.1", "2001:db8::/32")
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	ps, err := parseResolved("203.0.113.1,2001:db8::/32")
	if err != nil || joinPrefixes(ps) != "203.0.113.1,2001:db8::/32" {
		t.Fatalf("parseResolved = %v, %v", ps, err)
	}
	if _, err := parseResolved("example.com"); err == nil {
		t.Fatal("host name accepted as resolved")
	}
}

func TestParseResolvConf(t *testing.T) {
	got := parseResolvConf("# comment\nnameserver 127.0.0.53\nnameserver fe80::1%eth0\nsearch lan\nnameserver bogus\noptions edns0\n")
	want := []netip.Addr{netip.MustParseAddr("127.0.0.53"), netip.MustParseAddr("fe80::1")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestCheckProtected(t *testing.T) {
	ranges := protectedRanges([]netip.Addr{netip.MustParseAddr("1.1.1.1")})
	for _, s := range []string{"127.0.0.1", "1.1.1.0/24", "0.0.0.0/0", "255.255.255.255", "::1"} {
		if checkProtected(pfx(s)[0], ranges) == nil {
			t.Errorf("%s not protected", s)
		}
	}
	for _, s := range []string{"1.1.1.2", "8.8.8.8", "2001:db8::/32"} {
		if err := checkProtected(pfx(s)[0], ranges); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
}
