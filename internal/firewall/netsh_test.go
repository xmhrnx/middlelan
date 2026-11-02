package firewall

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
)

// forbiddenNetsh are netsh words that change profiles, policies or the
// firewall's state. No generated command may contain them.
var forbiddenNetsh = []string{"set", "reset", "firewallpolicy", "state", "allprofiles", "currentprofile", "import", "export", "dir=in", "action=allow", "name=all\"", "consec", "mainmode"}

func checkNetsh(t *testing.T, args []string) {
	t.Helper()
	line := strings.ToLower(strings.Join(args, " "))
	if !strings.HasPrefix(line, "advfirewall firewall ") {
		t.Errorf("not a firewall rule command: %s", line)
	}
	for _, f := range forbiddenNetsh {
		for _, a := range args {
			if strings.EqualFold(a, f) || strings.HasPrefix(strings.ToLower(a), f+"=") || strings.EqualFold(a, `name=all"`) {
				t.Errorf("command %q contains forbidden %q", line, f)
			}
		}
	}
	if !strings.Contains(line, " add rule ") && !strings.Contains(line, " delete rule ") && !strings.Contains(line, " show rule ") {
		t.Errorf("unexpected verb: %s", line)
	}
	if strings.Contains(line, " delete rule ") && !strings.Contains(line, `name="middlelan:`) {
		t.Errorf("delete of a rule that isn't MiddleLAN's: %s", line)
	}
}

func TestWindowsAddArgs(t *testing.T) {
	r := Rule{ID: "abc123", Exe: `C:\Program Files\Mozilla Firefox\firefox.exe`}
	ips := []netip.Prefix{netip.MustParsePrefix("93.184.216.34/32"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2606:2800::/32")}
	args := windowsAddArgs(r, ips)
	checkNetsh(t, args)
	line := strings.Join(args, " ")
	for _, want := range []string{
		`advfirewall firewall add rule name="MiddleLAN:abc123" dir=out action=block`,
		`program="C:\Program Files\Mozilla Firefox\firefox.exe"`,
		`remoteip=93.184.216.34,10.0.0.0/8,2606:2800::/32`,
		`enable=yes profile=any`,
		`description="MiddleLAN fp=`,
	} {
		if !strings.Contains(line, want) {
			t.Errorf("%s\nmissing %s", line, want)
		}
	}
	if !slices.Contains(args, "profile=any") {
		t.Error("profile=any missing")
	}

	app := windowsAddArgs(Rule{ID: "x", Exe: `C:\a.exe`}, nil)
	checkNetsh(t, app)
	if strings.Contains(strings.Join(app, " "), "remoteip") {
		t.Error("remoteip on an app-wide rule")
	}
	dest := windowsAddArgs(Rule{ID: "y"}, ips[:1])
	checkNetsh(t, dest)
	if strings.Contains(strings.Join(dest, " "), "program=") {
		t.Error("program on a destination rule")
	}
	if windowsFingerprint(r, ips) == windowsFingerprint(r, ips[:1]) {
		t.Error("fingerprint ignores addresses")
	}
	if windowsFingerprint(r, ips) != windowsFingerprint(Rule{ID: "other", Exe: strings.ToUpper(r.Exe)}, ips) {
		t.Error("fingerprint depends on ID or path case")
	}
}

func TestWindowsOtherArgs(t *testing.T) {
	checkNetsh(t, windowsDeleteArgs(windowsRuleName("abc")))
	checkNetsh(t, windowsShowArgs())
	got := netshCmdLine(`C:\Windows\System32\netsh.exe`, windowsDeleteArgs("MiddleLAN:abc"))
	want := `"C:\Windows\System32\netsh.exe" advfirewall firewall delete rule name="MiddleLAN:abc"`
	if got != want {
		t.Fatalf("cmdline = %s", got)
	}
}

const netshEnglish = "\r\n" + `Rule Name:                            Core Networking - DNS (UDP-Out)
----------------------------------------------------------------------
Description:                          Outbound rule
Enabled:                              Yes
Direction:                            Out

Rule Name:                            MiddleLAN:abc123
----------------------------------------------------------------------
Description:                          MiddleLAN fp=0123456789abcdef
Enabled:                              Yes
Direction:                            Out
Profiles:                             Domain,Private,Public
Program:                              C:\Program Files\MiddleLAN:x\app.exe
RemoteIP:                             93.184.216.34/32
Action:                               Block

Rule Name:                            MiddleLAN:old
----------------------------------------------------------------------
Enabled:                              Yes

Rule Name:                            MiddleLAN:dup
----------------------------------------------------------------------
Description:                          MiddleLAN fp=aaaa
Rule Name:                            Not MiddleLAN:mine
Description:                          user rule

Rule Name:                            MiddleLAN:dup
----------------------------------------------------------------------
Description:                          MiddleLAN fp=bbbb
Ok.
`

const netshGerman = `Regelname:                            MiddleLAN:de1
----------------------------------------------------------------------
Beschreibung:                         MiddleLAN fp=cccc
Aktiviert:                            Ja
`

func TestParseNetshRules(t *testing.T) {
	got := parseNetshRules(netshEnglish)
	want := map[string][]string{
		"MiddleLAN:abc123": {"0123456789abcdef"},
		"MiddleLAN:old":    {""},
		"MiddleLAN:dup":    {"aaaa", "bbbb"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	de := parseNetshRules(netshGerman)
	if !slices.Equal(de["MiddleLAN:de1"], []string{"cccc"}) {
		t.Fatalf("german: %v", de)
	}
	if len(parseNetshRules("No rules match the specified criteria.\r\n")) != 0 {
		t.Fatal("found rules in an empty listing")
	}
}

func TestWindowsPlan(t *testing.T) {
	ips := []netip.Prefix{netip.MustParsePrefix("203.0.113.1/32")}
	keep := windowsAdd{Rule{ID: "keep"}, ips}
	changed := windowsAdd{Rule{ID: "changed"}, ips}
	fresh := windowsAdd{Rule{ID: "new"}, ips}
	dup := windowsAdd{Rule{ID: "dup"}, ips}
	existing := map[string][]string{
		"MiddleLAN:keep":    {windowsFingerprint(keep.rule, keep.ips)},
		"MiddleLAN:changed": {"stale"},
		"MiddleLAN:dup":     {windowsFingerprint(dup.rule, dup.ips), windowsFingerprint(dup.rule, dup.ips)},
		"MiddleLAN:gone":    {""},
	}
	deletes, adds := windowsPlan([]windowsAdd{keep, changed, fresh, dup}, existing)
	if want := []string{"MiddleLAN:changed", "MiddleLAN:dup", "MiddleLAN:gone"}; !slices.Equal(deletes, want) {
		t.Fatalf("deletes = %v, want %v", deletes, want)
	}
	var ids []string
	for _, a := range adds {
		ids = append(ids, a.rule.ID)
	}
	if want := []string{"changed", "new", "dup"}; !slices.Equal(ids, want) {
		t.Fatalf("adds = %v, want %v", ids, want)
	}
	for _, d := range deletes {
		checkNetsh(t, windowsDeleteArgs(d))
	}
	// Clearing is a plan with nothing wanted: every MiddleLAN rule goes.
	deletes, adds = windowsPlan(nil, existing)
	if len(deletes) != 4 || len(adds) != 0 {
		t.Fatalf("clear plan: %v %v", deletes, adds)
	}
}
