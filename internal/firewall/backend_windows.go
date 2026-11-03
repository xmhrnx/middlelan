//go:build windows

package firewall

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// errWindowsAdmin is returned when MiddleMan isn't elevated.
var errWindowsAdmin = errors.New("changing Windows Firewall rules needs administrator rights: restart MiddleMan with \"Run as administrator\"")

// errWindowsServiceOff is returned when the Windows Firewall service is
// stopped. MiddleMan never starts it.
var errWindowsServiceOff = errors.New("the Windows Firewall service (MpsSvc) isn't running, so rules can't be added; MiddleMan won't start it for you")

// windowsBackend adds outbound block rules to Windows Defender Firewall
// with netsh. It never changes profiles or default policies.
type windowsBackend struct {
	mu sync.Mutex
}

// DefaultBackend returns the firewall backend for this platform.
func DefaultBackend() Backend { return &windowsBackend{} }

func (*windowsBackend) Name() string { return "Windows Firewall" }

func (*windowsBackend) Capabilities() Capabilities {
	return Capabilities{
		PerApp: true, PerDestination: true, NeedsAdmin: true, Backend: "Windows Firewall",
		Notes: "Rules are outbound block rules named \"MiddleLAN:…\" in Windows Defender Firewall. " +
			"Blocking a program needs its full path. Windows Firewall doesn't filter loopback traffic.",
	}
}

// Validate requires a full path for per-program rules.
func (*windowsBackend) Validate(r Rule) error {
	if !r.AnyApp() && r.Exe == "" {
		return fmt.Errorf("blocking %q on Windows needs the program's full path", r.App)
	}
	return nil
}

func (*windowsBackend) SystemFirewallOff() bool {
	if running, ok := firewallServiceRunning(); ok && !running {
		return true
	}
	return allProfilesOff()
}

func (b *windowsBackend) Apply(rules []Rule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errWindowsAdmin
	}
	if running, ok := firewallServiceRunning(); ok && !running {
		return errWindowsServiceOff
	}
	existing, err := listWindowsRules()
	if err != nil {
		return err
	}
	var want []windowsAdd
	var problems []Problem
	for _, r := range rules {
		ips, err := parseResolved(r.Dest)
		if err != nil {
			problems = append(problems, Problem{r.ID, err.Error()})
			continue
		}
		if r.AnyApp() && len(ips) == 0 {
			problems = append(problems, Problem{r.ID, ErrBlocksEverything.Error()})
			continue
		}
		want = append(want, windowsAdd{r, ips})
	}
	deletes, adds := windowsPlan(want, existing)
	var errs []error
	for _, name := range deletes {
		if _, err := runNetsh(windowsDeleteArgs(name)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("removing stale rules: %w", err)
	}
	for _, a := range adds {
		if _, err := runNetsh(windowsAddArgs(a.rule, a.ips)); err != nil {
			problems = append(problems, Problem{a.rule.ID, err.Error()})
		}
	}
	return partial(problems)
}

func (b *windowsBackend) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	existing, err := listWindowsRules()
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errWindowsAdmin
	}
	var errs []error
	for name := range existing {
		if _, err := runNetsh(windowsDeleteArgs(name)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	left, err := listWindowsRules()
	if err != nil {
		return err
	}
	if len(left) > 0 {
		return fmt.Errorf("%d MiddleMan rule(s) are still in Windows Firewall after removal", len(left))
	}
	return nil
}

// listWindowsRules returns MiddleMan's rules in Windows Firewall.
func listWindowsRules() (map[string][]string, error) {
	out, err := runNetsh(windowsShowArgs())
	if err != nil {
		return nil, fmt.Errorf("listing Windows Firewall rules: %w", err)
	}
	return parseNetshRules(out), nil
}
