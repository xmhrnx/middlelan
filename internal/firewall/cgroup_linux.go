//go:build linux && !android

package firewall

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// cgroupRoot is where nftables looks up cgroup v2 paths.
const cgroupRoot = "/sys/fs/cgroup"

// programCgroups finds the cgroups of the running instances of exe and
// keeps those dedicated to it. It returns an explanation when there is
// none, because the program can't then be blocked on its own.
func programCgroups(exe string) ([]cgroupRef, error) {
	if _, err := os.Stat(filepath.Join(cgroupRoot, "cgroup.controllers")); err != nil {
		return nil, fmt.Errorf("this system doesn't use a unified cgroup v2 hierarchy, so %s can't be blocked on its own", filepath.Base(exe))
	}
	target := exe
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		target = real
	}
	pids, err := filepath.Glob("/proc/[0-9]*")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, dir := range pids {
		if procExe(dir) != target {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "cgroup"))
		if err != nil {
			continue // the process exited
		}
		if p, ok := parseProcCgroup(string(b)); ok && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s isn't running, so it can't be blocked yet; it will be when the rules are next applied while it runs", filepath.Base(exe))
	}
	var refs []cgroupRef
	for _, p := range paths {
		if !validCgroupPath(p) {
			continue
		}
		if _, err := os.Stat(filepath.Join(cgroupRoot, p)); err != nil {
			continue
		}
		if dedicatedScope(p, target, cgroupMembers(p)) {
			refs = append(refs, cgroupRefFor(p))
		}
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("%s shares its cgroup with other programs on this system, so it can't be blocked individually (block a destination instead)", filepath.Base(exe))
	}
	return refs, nil
}

// procExe returns the executable of the process in a /proc/<pid> directory.
func procExe(dir string) string {
	exe, err := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(exe, " (deleted)")
}

// cgroupMembers returns the executables of the processes in a cgroup.
func cgroupMembers(path string) []string {
	b, err := os.ReadFile(filepath.Join(cgroupRoot, path, "cgroup.procs"))
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range strings.Fields(string(b)) {
		if _, err := strconv.Atoi(f); err != nil {
			continue
		}
		if exe := procExe(filepath.Join("/proc", f)); exe != "" {
			out = append(out, exe)
		}
	}
	return out
}
