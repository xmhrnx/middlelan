package firewall

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Options customizes how a Firewall resolves names. Zero fields use the
// system's resolver and DNS servers.
type Options struct {
	// LookupHost resolves a host name to addresses.
	LookupHost func(ctx context.Context, host string) ([]netip.Addr, error)
	// DNSServers returns the configured DNS servers, which rules may never
	// block.
	DNSServers func() []netip.Addr
	// LookupTimeout bounds each host name lookup.
	LookupTimeout time.Duration
}

// maxHostAddrs caps how many addresses of one host name are blocked.
const maxHostAddrs = 64

// offCacheFor is how long the backend's SystemFirewallOff answer is reused.
const offCacheFor = 10 * time.Second

// Firewall stores rules in dir/firewall.json and enforces them through a
// Backend. Its methods are safe from any goroutine.
type Firewall struct {
	// applyMu serializes changes to the file and the system, so the slow
	// parts (lookups, firewall commands) never hold mu.
	applyMu sync.Mutex

	mu      sync.Mutex
	path    string
	b       Backend
	opts    Options
	st      state
	hosts   map[string][]netip.Prefix // last resolution of each host name
	dns     []netip.Addr              // last known DNS servers
	applied int
	errs    []string

	offMu      sync.Mutex
	off        bool
	offChecked time.Time
}

// Open loads the rules in dir, using the system's resolver. It never
// changes the system: the firewall starts as the user left it, and rules
// are only (re)applied by Enable, Refresh or a rule change while enabled.
// On a read error Open returns a usable, empty firewall with the error.
func Open(dir string, b Backend) (*Firewall, error) {
	return OpenOptions(dir, b, Options{})
}

// OpenOptions is Open with a custom resolver.
func OpenOptions(dir string, b Backend, opts Options) (*Firewall, error) {
	if opts.LookupHost == nil {
		opts.LookupHost = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	if opts.DNSServers == nil {
		opts.DNSServers = systemDNSServers
	}
	if opts.LookupTimeout <= 0 {
		opts.LookupTimeout = 5 * time.Second
	}
	f := &Firewall{path: filepath.Join(dir, fileName), b: b, opts: opts, hosts: map[string][]netip.Prefix{}}
	st, err := loadState(f.path)
	if err != nil {
		return f, fmt.Errorf("reading firewall rules: %w", err)
	}
	f.st = st
	// Re-check rules from disk: a hand-edited file mustn't smuggle in a rule
	// that blocks everything or a protected address.
	caps := b.Capabilities()
	protected := f.protected()
	var kept []Rule
	var bad []error
	seen := map[string]bool{}
	for _, r := range st.Rules {
		if !validID(r.ID) || seen[r.ID] {
			id, err := newID()
			if err != nil {
				return f, err
			}
			r.ID = id
		}
		seen[r.ID] = true
		nr, _, _, err := normalizeRule(r, caps, b, protected)
		if err != nil {
			bad = append(bad, fmt.Errorf("dropped rule %s: %w", r.ID, err))
			continue
		}
		kept = append(kept, nr)
	}
	f.st.Rules = kept
	return f, errors.Join(bad...)
}

// Enabled reports whether the user has turned the firewall on.
func (f *Firewall) Enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st.Enabled
}

// Rules returns a copy of the rules.
func (f *Firewall) Rules() []Rule {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.st.Rules)
}

// Capabilities describes what the backend can enforce.
func (f *Firewall) Capabilities() Capabilities { return f.b.Capabilities() }

// Status describes the firewall's state as of the last apply.
func (f *Firewall) Status() Status {
	off := f.systemOff()
	f.mu.Lock()
	defer f.mu.Unlock()
	return Status{
		Enabled: f.st.Enabled, Applied: f.applied, Errors: slices.Clone(f.errs),
		Backend: f.b.Name(), SystemFirewallOff: off,
	}
}

// systemOff asks the backend whether the system firewall is off, at most
// every offCacheFor.
func (f *Firewall) systemOff() bool {
	f.offMu.Lock()
	defer f.offMu.Unlock()
	if time.Since(f.offChecked) > offCacheFor {
		f.off = f.b.SystemFirewallOff()
		f.offChecked = time.Now()
	}
	return f.off
}

// Enable turns the firewall on and applies the rules. Rules that can't be
// enforced don't make it fail: they are listed in Status().Errors. If the
// rules can't be applied at all, everything is removed again, the firewall
// stays off and the error is returned.
func (f *Firewall) Enable() error {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()
	f.mu.Lock()
	f.st.Enabled, f.st.Installed = true, true
	st := f.st.clone()
	f.mu.Unlock()
	if err := saveState(f.path, st); err != nil {
		f.setEnabled(false, st.Installed)
		return fmt.Errorf("saving firewall rules: %w", err)
	}
	err := f.apply(st.Rules)
	if err == nil {
		return nil
	}
	clearErr := f.b.Clear()
	f.setEnabled(false, clearErr != nil)
	if clearErr != nil {
		f.setErrs(fmt.Sprintf("couldn't remove partly applied rules: %v", clearErr))
	}
	f.mu.Lock()
	st = f.st.clone()
	f.mu.Unlock()
	saveErr := saveState(f.path, st)
	if clearErr != nil {
		clearErr = fmt.Errorf("removing partly applied rules: %w", clearErr)
	}
	return errors.Join(err, clearErr, saveErr)
}

// Disable turns the firewall off and removes every rule MiddleMan created,
// including leftovers from earlier runs. It can be called again after a
// failure.
func (f *Firewall) Disable() error {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()
	f.setEnabled(false, true)
	if err := f.saveCurrent(); err != nil {
		return err
	}
	return f.clear()
}

// Refresh re-resolves host names and applies the rules again when the
// firewall is on. When it is off it removes rules a crash may have left
// behind. Call it at startup and now and then, such as when the network
// changes; it does nothing on a firewall that was never enabled.
func (f *Firewall) Refresh() error {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()
	f.mu.Lock()
	st := f.st.clone()
	f.mu.Unlock()
	switch {
	case st.Enabled:
		return f.apply(st.Rules)
	case st.Installed:
		return f.clear()
	}
	return nil
}

// Uninstall removes every rule MiddleMan created and deletes the rules file.
// If the system rules can't be removed the file is kept, so Uninstall can be
// retried.
func (f *Firewall) Uninstall() error {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()
	if err := f.b.Clear(); err != nil {
		f.setErrs(err.Error())
		return fmt.Errorf("removing firewall rules: %w", err)
	}
	if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f.mu.Lock()
	f.st = state{}
	f.applied, f.errs = 0, nil
	f.hosts = map[string][]netip.Prefix{}
	f.mu.Unlock()
	return nil
}

// Add validates r, gives it a new ID and stores it, applying it at once if
// the firewall is on. If it can't be applied the rule isn't kept.
func (f *Firewall) Add(r Rule) (Rule, error) {
	r, err := f.check(r)
	if err != nil {
		return Rule{}, err
	}
	id, err := newID()
	if err != nil {
		return Rule{}, err
	}
	r.ID = id
	if r.Created.IsZero() {
		r.Created = time.Now()
	}
	err = f.change(func(st *state) error {
		st.Rules = append(st.Rules, r)
		return nil
	})
	if err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Update replaces the rule with r's ID, keeping its creation time.
func (f *Firewall) Update(r Rule) error {
	r, err := f.check(r)
	if err != nil {
		return err
	}
	return f.change(func(st *state) error {
		i := slices.IndexFunc(st.Rules, func(x Rule) bool { return x.ID == r.ID })
		if i < 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, r.ID)
		}
		r.Created = st.Rules[i].Created
		st.Rules[i] = r
		return nil
	})
}

// Remove deletes the rule with this ID.
func (f *Firewall) Remove(id string) error {
	return f.change(func(st *state) error {
		i := slices.IndexFunc(st.Rules, func(x Rule) bool { return x.ID == id })
		if i < 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		st.Rules = slices.Delete(st.Rules, i, i+1)
		return nil
	})
}

// check normalizes r and rejects it if it blocks everything or a protected
// address, including host names that resolve to one.
func (f *Firewall) check(r Rule) (Rule, error) {
	protected := f.protected()
	r, kind, _, err := normalizeRule(r, f.b.Capabilities(), f.b, protected)
	if err != nil {
		return r, err
	}
	if kind != destHost {
		return r, nil
	}
	addrs, err := f.lookup(r.Dest)
	if err != nil {
		// Offline: keep the rule; it is resolved again on every apply.
		return r, nil
	}
	for _, a := range addrs {
		if isProtectedAddr(a, protected) {
			return r, fmt.Errorf("%w: %s resolves to %s, which MiddleMan never blocks (loopback, DHCP or a DNS server)", ErrProtected, r.Dest, a)
		}
	}
	f.mu.Lock()
	f.hosts[r.Dest] = addrPrefixes(addrs)
	f.mu.Unlock()
	return r, nil
}

// change edits the rules, saves them and applies them if the firewall is
// on. If applying fails, the edit is undone.
func (f *Firewall) change(edit func(*state) error) error {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()
	f.mu.Lock()
	old := f.st.clone()
	next := f.st.clone()
	if err := edit(&next); err != nil {
		f.mu.Unlock()
		return err
	}
	f.st = next.clone()
	f.mu.Unlock()
	if err := saveState(f.path, next); err != nil {
		f.restore(old)
		return fmt.Errorf("saving firewall rules: %w", err)
	}
	if !next.Enabled {
		return nil
	}
	err := f.apply(next.Rules)
	if err == nil {
		return nil
	}
	f.restore(old)
	saveErr := saveState(f.path, old)
	reErr := f.apply(old.Rules)
	if reErr != nil {
		reErr = fmt.Errorf("restoring the previous rules: %w", reErr)
	}
	return errors.Join(err, saveErr, reErr)
}

// restore puts back an earlier state after a failed change.
func (f *Firewall) restore(st state) {
	f.mu.Lock()
	f.st = st
	f.mu.Unlock()
}

// apply resolves the enabled rules and hands them to the backend. Problems
// with single rules are recorded in the status; only a failure to apply at
// all is returned.
func (f *Firewall) apply(rules []Rule) error {
	resolved, hosts, problems := f.resolve(rules)
	err := f.b.Apply(resolved)
	f.mu.Lock()
	defer f.mu.Unlock()
	for h, ps := range hosts {
		f.hosts[h] = ps
	}
	var pe *PartialError
	switch {
	case err == nil:
		f.applied = len(resolved)
	case errors.As(err, &pe):
		skipped := map[string]bool{}
		for _, p := range pe.Problems {
			skipped[p.RuleID] = true
		}
		f.applied = 0
		for _, r := range resolved {
			if !skipped[r.ID] {
				f.applied++
			}
		}
		problems = append(problems, pe.Problems...)
		err = nil
	default:
		f.applied = 0
		f.errs = []string{err.Error()}
		return err
	}
	f.errs = nil
	for _, p := range problems {
		f.errs = append(f.errs, p.String())
	}
	return nil
}

// resolve turns the enabled rules into the form backends take, with host
// names replaced by addresses. Rules that would block everything or only
// protected addresses are left out and reported, never silently dropped.
func (f *Firewall) resolve(rules []Rule) ([]Rule, map[string][]netip.Prefix, []Problem) {
	protected := f.protected()
	var names []string
	for _, r := range rules {
		kind, _, host, err := parseDest(r.Dest)
		if err == nil && kind == destHost && r.Enabled && !slices.Contains(names, host) {
			names = append(names, host)
		}
	}
	hosts := map[string][]netip.Prefix{}
	lookupErr := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, host := range names {
		wg.Go(func() {
			addrs, err := f.lookup(host)
			mu.Lock()
			defer mu.Unlock()
			lookupErr[host] = err
			if err == nil {
				hosts[host] = addrPrefixes(addrs)
			}
		})
	}
	wg.Wait()
	var out []Rule
	var problems []Problem
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		kind, p, host, err := parseDest(r.Dest)
		if err != nil {
			problems = append(problems, Problem{r.ID, err.Error()})
			continue
		}
		if r.AnyApp() && kind == destAny {
			problems = append(problems, Problem{r.ID, ErrBlocksEverything.Error()})
			continue
		}
		var ps []netip.Prefix
		switch kind {
		case destPrefix:
			if err := checkProtected(p, protected); err != nil {
				problems = append(problems, Problem{r.ID, err.Error() + "; rule not applied"})
				continue
			}
			ps = []netip.Prefix{p}
		case destHost:
			if err := lookupErr[host]; err != nil {
				problems = append(problems, Problem{r.ID, fmt.Sprintf("couldn't resolve %s: %v; rule not applied", host, err)})
				continue
			}
			for _, q := range hosts[host] {
				if isProtectedAddr(q.Addr(), protected) {
					problems = append(problems, Problem{r.ID, fmt.Sprintf("%s resolves to %s, which MiddleMan never blocks; that address is left open", host, q.Addr())})
					continue
				}
				ps = append(ps, q)
			}
			if len(ps) == 0 {
				problems = append(problems, Problem{r.ID, fmt.Sprintf("%s has no blockable addresses; rule not applied", host)})
				continue
			}
		}
		r.Dest = joinPrefixes(minimalPrefixes(ps))
		out = append(out, r)
	}
	return out, hosts, problems
}

// protected reads the DNS servers, remembers them for Blocks and returns
// every range rules may not block.
func (f *Firewall) protected() []protectedRange {
	dns := f.opts.DNSServers()
	f.mu.Lock()
	f.dns = dns
	f.mu.Unlock()
	return protectedRanges(dns)
}

// lookup resolves a host name within the lookup timeout.
func (f *Firewall) lookup(host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.opts.LookupTimeout)
	defer cancel()
	addrs, err := f.opts.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	if len(addrs) > maxHostAddrs {
		addrs = addrs[:maxHostAddrs]
	}
	return addrs, nil
}

// addrPrefixes turns addresses into single-address prefixes.
func addrPrefixes(addrs []netip.Addr) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(addrs))
	for _, a := range addrs {
		a = a.Unmap().WithZone("")
		if a.IsValid() {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

// clear removes the backend's rules and, on success, records that nothing
// is installed any more.
func (f *Firewall) clear() error {
	if err := f.b.Clear(); err != nil {
		f.setErrs(fmt.Sprintf("couldn't remove MiddleMan's firewall rules: %v", err))
		return fmt.Errorf("removing firewall rules: %w", err)
	}
	f.mu.Lock()
	f.st.Installed = false
	f.applied, f.errs = 0, nil
	f.mu.Unlock()
	return f.saveCurrent()
}

// setEnabled sets the stored flags.
func (f *Firewall) setEnabled(enabled, installed bool) {
	f.mu.Lock()
	f.st.Enabled, f.st.Installed = enabled, installed
	f.mu.Unlock()
}

// setErrs replaces the status errors.
func (f *Firewall) setErrs(errs ...string) {
	f.mu.Lock()
	f.errs = errs
	f.mu.Unlock()
}

// saveCurrent writes the current state.
func (f *Firewall) saveCurrent() error {
	f.mu.Lock()
	st := f.st.clone()
	f.mu.Unlock()
	if err := saveState(f.path, st); err != nil {
		return fmt.Errorf("saving firewall rules: %w", err)
	}
	return nil
}
