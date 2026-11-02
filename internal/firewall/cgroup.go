package firewall

import (
	"path/filepath"
	"strings"
)

// parseProcCgroup returns the cgroup v2 path from /proc/<pid>/cgroup.
func parseProcCgroup(content string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "0::"); ok && strings.HasPrefix(rest, "/") {
			return rest, true
		}
	}
	return "", false
}

// cgroupRefFor turns an absolute cgroup path into the form nftables takes.
func cgroupRefFor(path string) cgroupRef {
	p := strings.Trim(path, "/")
	level := 0
	if p != "" {
		level = strings.Count(p, "/") + 1
	}
	return cgroupRef{Path: p, Level: level}
}

// exeBaseName returns a program's lower-case file name without extension.
func exeBaseName(exe string) string {
	base := strings.ToLower(filepath.Base(exe))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// dedicatedScope reports whether the cgroup at path holds only the program
// exe, so blocking the cgroup blocks just that program. A cgroup qualifies
// when its name contains the program's name, or when it is a desktop app
// scope ("app-…") whose members all run exe. Slices, the root and login
// sessions never qualify: they hold unrelated programs.
func dedicatedScope(path, exe string, members []string) bool {
	p := strings.Trim(path, "/")
	if p == "" {
		return false
	}
	last := strings.ToLower(p[strings.LastIndex(p, "/")+1:])
	if strings.HasSuffix(last, ".slice") || last == "init.scope" || strings.HasPrefix(last, "session-") {
		return false
	}
	if base := exeBaseName(exe); len(base) >= 3 && strings.Contains(last, base) {
		return true
	}
	if !strings.Contains(last, "app-") || len(members) == 0 {
		return false
	}
	for _, m := range members {
		if m != exe {
			return false
		}
	}
	return true
}

// validCgroupPath reports whether a cgroup path can be quoted safely.
func validCgroupPath(path string) bool {
	if path == "" || len(path) > 1024 {
		return false
	}
	for _, c := range path {
		if c == '"' || c == '\\' || c < ' ' || c == 0x7f {
			return false
		}
	}
	return true
}
