// Package firewall blocks chosen programs or destinations from using the
// network, through rules MiddleLAN adds to the operating system's own
// firewall.
//
// The firewall is off until the user turns it on: opening it never touches
// the system. It only ever adds narrowly scoped block rules that MiddleLAN
// owns and tags (Windows rules named "MiddleLAN:<id>", the nftables table
// "inet middlelan" on Linux, the pf anchor "com.apple/com.middlelan" on
// macOS). It never changes the system firewall's default policies or
// profiles, and never turns the system firewall on or off. Turning the
// firewall off removes every rule MiddleLAN created, including ones left over
// from an earlier run.
//
// Everything not blocked is allowed. Rules can't target loopback, DHCP or
// the configured DNS servers, and a rule must name a program or a
// destination: a rule that would block all traffic is rejected.
package firewall
