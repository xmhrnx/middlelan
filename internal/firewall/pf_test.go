package firewall

import (
	"strings"
	"testing"
)

func TestPfRules(t *testing.T) {
	got := pfRules([]pfEntry{
		{ID: "a", Dests: pfx("203.0.113.0/24", "203.0.113.7", "2001:db8::1")},
		{ID: "b", Dests: pfx("198.51.100.1")},
		{ID: "empty"},
	})
	want := `block drop out quick to { 203.0.113.0/24, 2001:db8::1 } label "MiddleLAN:a"
block drop out quick to { 198.51.100.1 } label "MiddleLAN:b"
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if !strings.HasPrefix(line, "block drop out quick to { ") || !strings.Contains(line, `label "MiddleLAN:`) {
			t.Errorf("unexpected pf rule %q", line)
		}
		for _, bad := range []string{"pass", "set ", " all", " any", "in ", "anchor", "scrub", "nat"} {
			if strings.Contains(line, bad) {
				t.Errorf("rule %q contains %q", line, bad)
			}
		}
	}
	if pfRules(nil) != "" || pfRules([]pfEntry{{ID: "x"}}) != "" {
		t.Fatal("rules without destinations")
	}
}

func TestPfArgs(t *testing.T) {
	for _, args := range [][]string{pfLoadArgs(), pfFlushArgs()} {
		if len(args) < 2 || args[0] != "-a" || args[1] != pfAnchor {
			t.Errorf("pfctl %v isn't limited to MiddleLAN's anchor", args)
		}
	}
	for _, args := range [][]string{pfLoadArgs(), pfFlushArgs(), pfInfoArgs()} {
		for _, a := range args {
			switch a {
			case "-e", "-E", "-d", "-X":
				t.Errorf("pfctl %v changes whether pf is enabled", args)
			}
		}
	}
	if !strings.HasPrefix(pfAnchor, "com.apple/") || !strings.Contains(pfAnchor, "middlelan") {
		t.Fatalf("anchor %q", pfAnchor)
	}
}

func TestPfEnabled(t *testing.T) {
	on, ok := pfEnabled("No ALTQ support in kernel\nStatus: Enabled for 0 days 00:01:02           Debug: Urgent\n")
	if !on || !ok {
		t.Fatal("enabled not detected")
	}
	on, ok = pfEnabled("Status: Disabled for 0 days\n")
	if on || !ok {
		t.Fatal("disabled not detected")
	}
	if _, ok := pfEnabled("garbage"); ok {
		t.Fatal("parsed garbage")
	}
}
