//go:build linux && !android

package firewall

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestNftSmoke applies and clears real nftables rules. It changes the
// system's firewall, so it only runs as root with MIDDLELAN_NFT_SMOKE=1.
// It blocks 192.0.2.123 (TEST-NET-1), which nothing uses.
func TestNftSmoke(t *testing.T) {
	if os.Getenv("MIDDLELAN_NFT_SMOKE") != "1" || os.Geteuid() != 0 {
		t.Skip("set MIDDLELAN_NFT_SMOKE=1 and run as root to test against nftables")
	}
	nft, err := findTool("nft")
	if err != nil {
		t.Skip(err)
	}
	ruleset := func() string {
		out, err := exec.Command(nft, "list", "ruleset").CombinedOutput()
		if err != nil {
			t.Fatalf("nft list ruleset: %v: %s", err, out)
		}
		return string(out)
	}
	before := ruleset()
	if strings.Contains(before, "table inet middlelan") {
		t.Fatal("MiddleLAN's table exists before the test")
	}
	b := DefaultBackend()
	defer b.Clear()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	err = b.Apply([]Rule{
		{ID: "smoke", Dest: "192.0.2.123", Enabled: true},
		{ID: "smoke6", Dest: "2001:db8::123", Enabled: true},
		{ID: "self", App: "test", Exe: self, Enabled: true},
	})
	var pe *PartialError
	if err != nil && !errors.As(err, &pe) {
		t.Fatalf("Apply: %v", err)
	}
	if pe != nil {
		t.Logf("partial (expected for the per-program rule on some systems): %v", pe)
		for _, p := range pe.Problems {
			if p.RuleID != "self" {
				t.Errorf("destination rule skipped: %v", p)
			}
		}
	}
	during := ruleset()
	t.Logf("table:\n%s", during[strings.Index(during, "table inet middlelan"):])
	for _, want := range []string{"192.0.2.123", "2001:db8::123", `comment "MiddleLAN:smoke"`, "policy accept"} {
		if !strings.Contains(during, want) {
			t.Errorf("ruleset lacks %q", want)
		}
	}
	if !strings.HasPrefix(during, before) && before != "" {
		t.Error("other tables changed")
	}

	// A locally generated packet dropped on output fails at once.
	conn, err := net.DialTimeout("tcp", "192.0.2.123:80", 2*time.Second)
	if err == nil {
		conn.Close()
		t.Error("connection to a blocked address succeeded")
	} else {
		t.Logf("dial blocked address: %v", err)
	}

	// Re-applying a smaller set removes stale rules.
	if err := b.Apply([]Rule{{ID: "smoke", Dest: "192.0.2.123", Enabled: true}}); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	if strings.Contains(ruleset(), "2001:db8::123") {
		t.Error("stale rule kept")
	}

	if err := b.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := b.Clear(); err != nil {
		t.Fatalf("second Clear: %v", err)
	}
	after := ruleset()
	if strings.Contains(after, "middlelan") {
		t.Fatalf("table left after Clear:\n%s", after)
	}
	if after != before {
		t.Fatalf("ruleset changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
