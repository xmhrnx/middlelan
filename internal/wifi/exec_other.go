//go:build !windows

package wifi

import "os/exec"

// hideWindow is a no-op outside Windows.
func hideWindow(*exec.Cmd) {}
