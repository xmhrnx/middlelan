package firewall

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

// Windows Firewall rules created by MiddleLAN are named rulePrefix+ID
// and described windowsTag+" fp=<fingerprint>", so they can be found and
// compared with the wanted rules.
const (
	rulePrefix = "MiddleLAN:"
	windowsTag = "MiddleLAN"
)

var (
	netshNameLine = regexp.MustCompile(`^[^:]*:\s*(` + rulePrefix + `[A-Za-z0-9_-]+)\s*$`)
	netshDescLine = regexp.MustCompile(`^[^:]*:\s*` + windowsTag + ` fp=([0-9a-f]+)\s*$`)
)

// windowsRuleName returns the Windows Firewall rule name for a rule ID.
func windowsRuleName(id string) string { return rulePrefix + id }

// windowsFingerprint summarizes what a rule enforces, so an unchanged rule
// is left in place on the next apply.
func windowsFingerprint(r Rule, ips []netip.Prefix) string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(r.Exe)))
	h.Write([]byte{0})
	h.Write([]byte(joinPrefixes(ips)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// windowsAddArgs returns the netsh arguments that add one outbound block
// rule. Values with spaces are quoted the way netsh expects, so the
// arguments must be joined into a raw command line (see netshCmdLine), not
// escaped again. Only "firewall add rule" is ever generated: profiles and
// default policies are never touched.
func windowsAddArgs(r Rule, ips []netip.Prefix) []string {
	args := []string{
		"advfirewall", "firewall", "add", "rule",
		`name="` + windowsRuleName(r.ID) + `"`,
		"dir=out", "action=block",
	}
	if r.Exe != "" {
		args = append(args, `program="`+r.Exe+`"`)
	}
	if len(ips) > 0 {
		args = append(args, "remoteip="+joinPrefixes(ips))
	}
	return append(args, "enable=yes", "profile=any",
		`description="`+windowsTag+` fp=`+windowsFingerprint(r, ips)+`"`)
}

// windowsDeleteArgs returns the netsh arguments that delete every rule with
// this name, which must be one of MiddleLAN's.
func windowsDeleteArgs(name string) []string {
	return []string{"advfirewall", "firewall", "delete", "rule", `name="` + name + `"`}
}

// windowsShowArgs returns the netsh arguments that list every rule.
func windowsShowArgs() []string {
	return []string{"advfirewall", "firewall", "show", "rule", "name=all", "verbose"}
}

// netshCmdLine joins netsh arguments into a Windows command line.
func netshCmdLine(exe string, args []string) string {
	return `"` + exe + `" ` + strings.Join(args, " ")
}

// parseNetshRules finds MiddleLAN's rules in "netsh advfirewall firewall
// show rule name=all verbose" output, mapping each name to the fingerprints
// of the rules with that name ("" when a rule has none). It matches values,
// not the (translated) field labels.
func parseNetshRules(out string) map[string][]string {
	rules := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.TrimSpace(line) == "" {
			cur = ""
			continue
		}
		if m := netshNameLine.FindStringSubmatch(line); m != nil {
			cur = m[1]
			rules[cur] = append(rules[cur], "")
			continue
		}
		if m := netshDescLine.FindStringSubmatch(line); m != nil && cur != "" {
			fps := rules[cur]
			fps[len(fps)-1] = m[1]
		}
	}
	return rules
}

// windowsAdd is a rule to create, with its addresses.
type windowsAdd struct {
	rule Rule
	ips  []netip.Prefix
}

// windowsPlan compares the wanted rules with the existing ones and returns
// the names to delete and the rules to add. A rule whose single existing
// copy has the right fingerprint is kept; anything else with MiddleLAN's
// prefix is deleted.
func windowsPlan(want []windowsAdd, existing map[string][]string) (deletes []string, adds []windowsAdd) {
	wanted := map[string]bool{}
	for _, w := range want {
		name := windowsRuleName(w.rule.ID)
		wanted[name] = true
		fps, ok := existing[name]
		if ok && len(fps) == 1 && fps[0] == windowsFingerprint(w.rule, w.ips) {
			continue
		}
		if ok {
			deletes = append(deletes, name)
		}
		adds = append(adds, w)
	}
	for name := range existing {
		if !wanted[name] && strings.HasPrefix(name, rulePrefix) {
			deletes = append(deletes, name)
		}
	}
	slices.Sort(deletes)
	return deletes, adds
}
