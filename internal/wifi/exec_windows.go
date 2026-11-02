//go:build windows

package wifi

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow keeps a console window from flashing up when the GUI runs a
// console tool.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
