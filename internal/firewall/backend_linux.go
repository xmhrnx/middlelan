//go:build linux && !android

package firewall

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// errLinuxAdmin is returned when nft is refused for lack of privileges.
var errLinuxAdmin = errors.New("changing nftables rules needs root (or CAP_NET_ADMIN): run MiddleMan with administrator rights")

// nftBackend enforces rules in MiddleMan's own nftables table. It never
// touches other tables, chains or policies.
type nftBackend struct {
	mu sync.Mutex
}

// DefaultBackend returns the firewall backend for this platform.
func DefaultBackend() Backend { return &nftBackend{} }

func (*nftBackend) Name() string { return "nftables" }

func (*nftBackend) Capabilities() Capabilities {
	c := Capabilities{
		PerApp: true, PerDestination: true, NeedsAdmin: true, Backend: "nftables",
		Notes: "Rules live in the nftables table \"inet middlelan\". A program can be blocked only while it runs " +
			"in a cgroup of its own, as apps started from the desktop usually do; rules are refreshed whenever " +
			"MiddleMan applies them again.",
	}
	if _, err := findTool("nft"); err != nil {
		c.Backend = "nftables (not installed)"
		c.Notes = "Install nftables (the nft command) to use MiddleMan's firewall."
	}
	return c
}

// Validate requires a full path for per-program rules.
func (*nftBackend) Validate(r Rule) error {
	if !r.AnyApp() && r.Exe == "" {
		return fmt.Errorf("blocking %q on Linux needs the program's full path", r.App)
	}
	return nil
}

// SystemFirewallOff is always false: MiddleMan's table works whether or not
// another firewall front end is active.
func (*nftBackend) SystemFirewallOff() bool { return false }

func (b *nftBackend) Apply(rules []Rule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	nft, err := findTool("nft")
	if err != nil {
		return fmt.Errorf("nftables isn't available: %w", err)
	}
	var entries []nftEntry
	var problems []Problem
	for _, r := range rules {
		dests, err := parseResolved(r.Dest)
		if err != nil {
			problems = append(problems, Problem{r.ID, err.Error()})
			continue
		}
		e := nftEntry{ID: r.ID, Dests: dests}
		if !r.AnyApp() {
			cgs, err := programCgroups(r.Exe)
			if err != nil {
				problems = append(problems, Problem{r.ID, err.Error()})
				continue
			}
			e.Cgroups = cgs
		} else if len(dests) == 0 {
			problems = append(problems, Problem{r.ID, ErrBlocksEverything.Error()})
			continue
		}
		entries = append(entries, e)
	}
	dns := systemDNSServers()
	err = runNft(nft, nftScript(entries, dns))
	if err == nil {
		return partial(problems)
	}
	// A program's cgroup can vanish between the scan and the load, or the
	// kernel may lack cgroup matching; keep the destination rules then.
	var dests []nftEntry
	for _, e := range entries {
		if len(e.Cgroups) == 0 {
			dests = append(dests, e)
		} else {
			problems = append(problems, Problem{e.ID, "couldn't add a per-program rule: " + err.Error()})
		}
	}
	if len(dests) == len(entries) || errors.Is(err, errLinuxAdmin) {
		return err
	}
	if err2 := runNft(nft, nftScript(dests, dns)); err2 != nil {
		return err
	}
	return partial(problems)
}

func (b *nftBackend) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	nft, err := findTool("nft")
	if err != nil {
		// Without nft no table could have been created.
		return nil
	}
	return runNft(nft, nftClearScript())
}

// runNft loads a script atomically.
func runNft(nft, script string) error {
	out, err := runTool(script, nft, "-f", "-")
	if err != nil && (strings.Contains(out, "Operation not permitted") || strings.Contains(out, "Permission denied")) {
		return errLinuxAdmin
	}
	return err
}
