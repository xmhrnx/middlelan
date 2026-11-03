package firewall

import (
	"fmt"
	"net/netip"
	"strings"
)

// nftTable is MiddleMan's own nftables table. Nothing outside it is ever
// changed.
const nftTable = "inet middlelan"

// nftEntry is one rule ready for nftables.
type nftEntry struct {
	// ID is the rule's ID, kept in each nftables rule's comment.
	ID string
	// Cgroups are the program's cgroups; nil means any program.
	Cgroups []cgroupRef
	// Dests are the blocked addresses; nil means every destination.
	Dests []netip.Prefix
}

// cgroupRef is a cgroup v2 path relative to the hierarchy's root.
type cgroupRef struct {
	// Path has no leading slash, such as "user.slice/app-firefox.scope".
	Path string
	// Level is the path's depth.
	Level int
}

// nftScript returns an nft -f script that atomically replaces MiddleMan's
// table with one holding these rules. The table's chain accepts by default
// and only drops matching traffic: loopback, DHCP and DNS to the given
// servers are accepted first, so they are never dropped. Other tables and
// their policies are never touched.
func nftScript(entries []nftEntry, dns []netip.Addr) string {
	var b strings.Builder
	// Adding then deleting makes the delete safe when the table is missing;
	// all of it is one transaction.
	fmt.Fprintf(&b, "add table %s\ndelete table %s\n", nftTable, nftTable)
	fmt.Fprintf(&b, "table %s {\n\tchain output {\n", nftTable)
	b.WriteString("\t\ttype filter hook output priority 0; policy accept;\n")
	b.WriteString("\t\toifname \"lo\" accept comment \"MiddleLAN: loopback is never blocked\"\n")
	b.WriteString("\t\tudp dport { 67, 547 } accept comment \"MiddleLAN: DHCP is never blocked\"\n")
	var dnsPrefixes []netip.Prefix
	for _, a := range dns {
		if a = a.Unmap().WithZone(""); a.IsValid() {
			dnsPrefixes = append(dnsPrefixes, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	v4, v6 := splitFamilies(minimalPrefixes(dnsPrefixes))
	if len(v4) > 0 {
		fmt.Fprintf(&b, "\t\tip daddr %s meta l4proto { tcp, udp } th dport 53 accept comment \"MiddleLAN: DNS servers are never blocked\"\n", nftSet(v4))
	}
	if len(v6) > 0 {
		fmt.Fprintf(&b, "\t\tip6 daddr %s meta l4proto { tcp, udp } th dport 53 accept comment \"MiddleLAN: DNS servers are never blocked\"\n", nftSet(v6))
	}
	for _, e := range entries {
		for _, line := range nftRuleLines(e) {
			fmt.Fprintf(&b, "\t\t%s\n", line)
		}
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

// nftRuleLines returns the drop rules for one entry. An entry for any
// program with no destination would drop everything, so it yields nothing.
func nftRuleLines(e nftEntry) []string {
	if len(e.Cgroups) == 0 && len(e.Dests) == 0 {
		return nil
	}
	matchers := []string{""}
	if len(e.Cgroups) > 0 {
		matchers = matchers[:0]
		for _, c := range e.Cgroups {
			matchers = append(matchers, fmt.Sprintf("socket cgroupv2 level %d %q ", c.Level, c.Path))
		}
	}
	comment := fmt.Sprintf("comment %q", rulePrefix+e.ID)
	v4, v6 := splitFamilies(minimalPrefixes(e.Dests))
	var out []string
	for _, m := range matchers {
		if len(e.Dests) == 0 {
			out = append(out, m+"drop "+comment)
			continue
		}
		if len(v4) > 0 {
			out = append(out, m+"ip daddr "+nftSet(v4)+" drop "+comment)
		}
		if len(v6) > 0 {
			out = append(out, m+"ip6 daddr "+nftSet(v6)+" drop "+comment)
		}
	}
	return out
}

// nftSet formats prefixes as an anonymous nftables set.
func nftSet(ps []netip.Prefix) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = prefixString(p)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// nftClearScript returns an nft -f script that deletes MiddleMan's table
// and succeeds whether or not it exists.
func nftClearScript() string {
	return fmt.Sprintf("add table %s\ndelete table %s\n", nftTable, nftTable)
}
