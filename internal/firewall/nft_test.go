package firewall

import (
	"net/netip"
	"strings"
	"testing"
)

func pfx(ss ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range ss {
		_, p, _, err := parseDest(s)
		if err != nil {
			panic(err)
		}
		out = append(out, p)
	}
	return out
}

// checkNftSafe asserts a script only touches MiddleLAN's table and never
// changes a policy to drop.
func checkNftSafe(t *testing.T, script string) {
	t.Helper()
	lower := strings.ToLower(script)
	for _, bad := range []string{"policy drop", "flush ruleset", "delete table ip ", "delete table inet filter", "chain input", "hook input", "hook forward", "reject"} {
		if strings.Contains(lower, bad) {
			t.Errorf("script contains %q:\n%s", bad, script)
		}
	}
	for _, line := range strings.Split(script, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && (f[0] == "table" || f[0] == "add" || f[0] == "delete" || f[0] == "flush") {
			if !strings.Contains(line, nftTable) {
				t.Errorf("line touches another table: %q", line)
			}
		}
		if strings.Contains(line, "policy") && !strings.Contains(line, "policy accept;") {
			t.Errorf("unexpected policy: %q", line)
		}
		if strings.HasSuffix(strings.TrimSpace(line), "drop") || strings.Contains(line, " drop ") {
			if !strings.Contains(line, `comment "MiddleLAN:`) {
				t.Errorf("untagged drop rule: %q", line)
			}
		}
	}
}

func TestNftScript(t *testing.T) {
	entries := []nftEntry{
		{ID: "d1", Dests: pfx("203.0.113.0/24", "203.0.113.5", "2001:db8::/32")},
		{ID: "a1", Cgroups: []cgroupRef{cgroupRefFor("/user.slice/user-1000.slice/user@1000.service/app.slice/app-firefox-1.scope")}},
		{ID: "a2", Cgroups: []cgroupRef{{"system.slice/nginx.service", 2}}, Dests: pfx("198.51.100.1")},
		{ID: "everything"}, // must be ignored
	}
	dns := []netip.Addr{netip.MustParseAddr("192.168.1.1"), netip.MustParseAddr("2001:db8::53"), netip.MustParseAddr("::ffff:10.0.0.1")}
	s := nftScript(entries, dns)
	checkNftSafe(t, s)
	want := `add table inet middlelan
delete table inet middlelan
table inet middlelan {
	chain output {
		type filter hook output priority 0; policy accept;
		oifname "lo" accept comment "MiddleLAN: loopback is never blocked"
		udp dport { 67, 547 } accept comment "MiddleLAN: DHCP is never blocked"
		ip daddr { 10.0.0.1, 192.168.1.1 } meta l4proto { tcp, udp } th dport 53 accept comment "MiddleLAN: DNS servers are never blocked"
		ip6 daddr { 2001:db8::53 } meta l4proto { tcp, udp } th dport 53 accept comment "MiddleLAN: DNS servers are never blocked"
		ip daddr { 203.0.113.0/24 } drop comment "MiddleLAN:d1"
		ip6 daddr { 2001:db8::/32 } drop comment "MiddleLAN:d1"
		socket cgroupv2 level 5 "user.slice/user-1000.slice/user@1000.service/app.slice/app-firefox-1.scope" drop comment "MiddleLAN:a1"
		socket cgroupv2 level 2 "system.slice/nginx.service" ip daddr { 198.51.100.1 } drop comment "MiddleLAN:a2"
	}
}
`
	if s != want {
		t.Fatalf("script:\n%s\nwant:\n%s", s, want)
	}
	if strings.Contains(s, "everything") {
		t.Fatal("block-everything entry generated a rule")
	}
}

func TestNftScriptEmpty(t *testing.T) {
	s := nftScript(nil, nil)
	checkNftSafe(t, s)
	if strings.Contains(s, " drop") || strings.Contains(s, "th dport 53") {
		t.Fatalf("empty script has rules:\n%s", s)
	}
	c := nftClearScript()
	checkNftSafe(t, c)
	if c != "add table inet middlelan\ndelete table inet middlelan\n" {
		t.Fatalf("clear script = %q", c)
	}
}
