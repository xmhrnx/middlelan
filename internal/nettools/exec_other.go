//go:build !windows

package nettools

import "os/exec"

// hideWindow is a no-op outside Windows.
func hideWindow(*exec.Cmd) {}
