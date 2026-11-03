//go:build darwin && !ios

package firewall

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	// errDarwinAdmin is returned when MiddleMan doesn't run as root.
	errDarwinAdmin = errors.New("changing pf rules needs administrator (root) rights")
	// errPFDisabled is returned when pf is off. MiddleMan never enables it.
	errPFDisabled = errors.New("the macOS packet filter (pf) is disabled, so blocking rules wouldn't take effect; MiddleMan won't turn it on for you (an administrator can run \"sudo pfctl -E\")")
)

// pfBackend enforces destination rules in MiddleMan's pf anchor. macOS pf
// can't tell programs apart, so rules can't target a program.
type pfBackend struct {
	mu         sync.Mutex
	off        bool
	offChecked time.Time
}

// DefaultBackend returns the firewall backend for this platform.
func DefaultBackend() Backend { return &pfBackend{} }

func (*pfBackend) Name() string { return "pf" }

func (*pfBackend) Capabilities() Capabilities {
	return Capabilities{
		PerApp: false, PerDestination: true, NeedsAdmin: true, Backend: "pf",
		Notes: "Rules block destinations for every program, in the pf anchor \"" + pfAnchor + "\". " +
			"macOS pf can't block a single program, and pf must already be enabled.",
	}
}

// SystemFirewallOff reports whether pf is disabled. Without root the state
// can't be read and false is returned.
func (b *pfBackend) SystemFirewallOff() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.offChecked) < 10*time.Second {
		return b.off
	}
	b.offChecked = time.Now()
	b.off = false
	if os.Geteuid() != 0 {
		return false
	}
	if enabled, err := b.enabled(); err == nil {
		b.off = !enabled
	}
	return b.off
}

func (b *pfBackend) Apply(rules []Rule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if os.Geteuid() != 0 {
		return errDarwinAdmin
	}
	enabled, err := b.enabled()
	if err != nil {
		return err
	}
	if !enabled {
		b.off, b.offChecked = true, time.Now()
		return errPFDisabled
	}
	var entries []pfEntry
	var problems []Problem
	for _, r := range rules {
		dests, err := parseResolved(r.Dest)
		switch {
		case err != nil:
			problems = append(problems, Problem{r.ID, err.Error()})
		case !r.AnyApp():
			problems = append(problems, Problem{r.ID, "pf can't block a single program on macOS"})
		case len(dests) == 0:
			problems = append(problems, Problem{r.ID, ErrBlocksEverything.Error()})
		default:
			entries = append(entries, pfEntry{r.ID, dests})
		}
	}
	pfctl, err := findTool("pfctl")
	if err != nil {
		return err
	}
	script := pfRules(entries)
	if script == "" {
		_, err = runTool("", pfctl, pfFlushArgs()...)
	} else {
		_, err = runTool(script, pfctl, pfLoadArgs()...)
	}
	if err != nil {
		return err
	}
	return partial(problems)
}

func (b *pfBackend) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	pfctl, err := findTool("pfctl")
	if err != nil {
		return nil // without pfctl no anchor could have been loaded
	}
	if os.Geteuid() != 0 {
		return errDarwinAdmin
	}
	_, err = runTool("", pfctl, pfFlushArgs()...)
	return err
}

// enabled asks pfctl whether pf is enabled.
func (b *pfBackend) enabled() (bool, error) {
	pfctl, err := findTool("pfctl")
	if err != nil {
		return false, err
	}
	out, err := runTool("", pfctl, pfInfoArgs()...)
	if err != nil {
		return false, err
	}
	on, ok := pfEnabled(out)
	if !ok {
		return false, fmt.Errorf("couldn't read pf's state from pfctl")
	}
	return on, nil
}
