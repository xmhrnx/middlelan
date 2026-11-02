package firewall

import (
	"net/netip"
	"path/filepath"
	"runtime"
	"strings"
)

// Blocks reports whether the enabled rules block traffic from a program to
// ip. app is the program's display name and exe its executable path; either
// may be "". It only checks rules in memory (host names use their last
// resolution), so it is cheap enough for every packet of the Android VPN.
// It is false while the firewall is off and for loopback, DHCP and the DNS
// servers, which are never blocked.
func (f *Firewall) Blocks(app, exe, ip string) bool {
	a, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	a = a.Unmap().WithZone("")
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.st.Enabled {
		return false
	}
	if isProtectedAddr(a, alwaysProtected) || f.isDNSServer(a) {
		return false
	}
	for _, r := range f.st.Rules {
		if r.Enabled && appMatches(r, app, exe) && f.destMatches(r, a) {
			return true
		}
	}
	return false
}

// isDNSServer reports whether a is one of the DNS servers. It is called with
// f.mu held.
func (f *Firewall) isDNSServer(a netip.Addr) bool {
	for _, d := range f.dns {
		if d.Unmap() == a {
			return true
		}
	}
	return false
}

// destMatches reports whether r's destination includes a. It is called with
// f.mu held.
func (f *Firewall) destMatches(r Rule, a netip.Addr) bool {
	kind, p, host, err := parseDest(r.Dest)
	if err != nil {
		return false
	}
	switch kind {
	case destAny:
		return !r.AnyApp()
	case destPrefix:
		return p.Contains(a)
	default:
		for _, q := range f.hosts[host] {
			if q.Contains(a) {
				return true
			}
		}
		return false
	}
}

// appMatches reports whether r applies to the program. The executable path
// decides when both sides have one; otherwise the display name does.
func appMatches(r Rule, app, exe string) bool {
	if r.AnyApp() {
		return true
	}
	if r.Exe != "" && exe != "" {
		return sameExe(r.Exe, exe)
	}
	return r.App != "" && app != "" && strings.EqualFold(r.App, app)
}

// sameExe compares executable paths, ignoring case where the file system
// usually does.
func sameExe(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
