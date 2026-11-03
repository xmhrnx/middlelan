package firewall

import (
	"fmt"
	"net/netip"
	"strings"
)

// pfAnchor is MiddleMan's pf anchor. macOS's /etc/pf.conf evaluates every
// anchor under "com.apple/", so rules there take effect without editing the
// main ruleset; nothing outside the anchor is ever changed.
const pfAnchor = "com.apple/com.middlelan"

// pfEntry is one destination rule ready for pf.
type pfEntry struct {
	// ID is the rule's ID, kept in the pf rule's label.
	ID string
	// Dests are the blocked addresses; never empty.
	Dests []netip.Prefix
}

// pfRules returns the anchor's ruleset: one quick outbound block rule per
// entry. It has no pass rules, options or "block all": loading it can't
// change how pf treats anything else. Entries without destinations are
// left out, since they would block everything.
func pfRules(entries []pfEntry) string {
	var b strings.Builder
	for _, e := range entries {
		ps := minimalPrefixes(e.Dests)
		if len(ps) == 0 {
			continue
		}
		parts := make([]string, len(ps))
		for i, p := range ps {
			parts[i] = prefixString(p)
		}
		fmt.Fprintf(&b, "block drop out quick to { %s } label %q\n", strings.Join(parts, ", "), rulePrefix+e.ID)
	}
	return b.String()
}

// pfLoadArgs returns the pfctl arguments that replace the anchor's rules
// with the ruleset on stdin.
func pfLoadArgs() []string { return []string{"-a", pfAnchor, "-f", "-"} }

// pfFlushArgs returns the pfctl arguments that empty the anchor. Flushing
// an anchor only removes its own rules and tables.
func pfFlushArgs() []string { return []string{"-a", pfAnchor, "-F", "all"} }

// pfInfoArgs returns the pfctl arguments that show whether pf is enabled.
func pfInfoArgs() []string { return []string{"-s", "info"} }

// pfEnabled parses "pfctl -s info" output.
func pfEnabled(info string) (enabled, ok bool) {
	for _, line := range strings.Split(info, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "Status:"); found {
			f := strings.Fields(rest)
			if len(f) > 0 {
				return f[0] == "Enabled", true
			}
		}
	}
	return false, false
}
