package firewall

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// maxNote is the longest note kept with a rule.
const maxNote = 500

// newID returns a random rule ID.
func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// validID reports whether id is safe to embed in rule names and comments.
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// checkText rejects quotes and control characters, which could break out of
// a quoted argument of a firewall command.
func checkText(field, s string) error {
	for _, c := range s {
		if c == '"' || unicode.IsControl(c) {
			return fmt.Errorf("%s can't contain quotes or control characters", field)
		}
	}
	return nil
}

// normalizeRule trims r, checks it doesn't block everything or a protected
// address, and checks the backend can enforce it. It doesn't resolve host
// names. The returned prefix is set when Dest is an address or prefix.
func normalizeRule(r Rule, caps Capabilities, b Backend, protected []protectedRange) (Rule, destKind, netip.Prefix, error) {
	r.App = strings.TrimSpace(r.App)
	r.Exe = strings.TrimSpace(r.Exe)
	r.Note = strings.TrimSpace(r.Note)
	if len(r.Note) > maxNote {
		r.Note = r.Note[:maxNote]
	}
	for _, f := range []struct{ name, val string }{{"the program name", r.App}, {"the program path", r.Exe}, {"the destination", r.Dest}} {
		if err := checkText(f.name, f.val); err != nil {
			return r, 0, netip.Prefix{}, err
		}
	}
	if r.Exe != "" && runtime.GOOS != "android" && !filepath.IsAbs(r.Exe) {
		return r, 0, netip.Prefix{}, fmt.Errorf("the program path %q must be a full path", r.Exe)
	}
	kind, p, dest, err := parseDest(r.Dest)
	if err != nil {
		return r, 0, netip.Prefix{}, err
	}
	r.Dest = dest
	if r.AnyApp() && kind == destAny {
		return r, 0, netip.Prefix{}, ErrBlocksEverything
	}
	if !r.AnyApp() && !caps.PerApp {
		return r, 0, netip.Prefix{}, fmt.Errorf("%s can't block individual programs; set a destination and no program instead", caps.Backend)
	}
	if kind != destAny && !caps.PerDestination {
		return r, 0, netip.Prefix{}, fmt.Errorf("%s can't block destinations", caps.Backend)
	}
	if kind == destPrefix {
		if err := checkProtected(p, protected); err != nil {
			return r, 0, netip.Prefix{}, err
		}
	}
	if v, ok := b.(Validator); ok {
		if err := v.Validate(r); err != nil {
			return r, 0, netip.Prefix{}, err
		}
	}
	return r, kind, p, nil
}
