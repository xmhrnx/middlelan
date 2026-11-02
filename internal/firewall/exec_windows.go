//go:build windows

package firewall

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// createNoWindow keeps a console window from flashing up for netsh.
const createNoWindow = 0x08000000

// netshTimeout bounds one netsh run.
const netshTimeout = 30 * time.Second

// netshPath returns netsh.exe in the system directory.
func netshPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", "netsh.exe")
	}
	return `C:\Windows\System32\netsh.exe`
}

// runNetsh runs netsh with pre-quoted arguments and a hidden window, and
// returns its output. A failure includes netsh's message.
func runNetsh(args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), netshTimeout)
	defer cancel()
	exe := netshPath()
	cmd := exec.CommandContext(ctx, exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
		CmdLine:       netshCmdLine(exe, args),
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return string(out), fmt.Errorf("netsh %s: %w", strings.Join(args[:min(4, len(args))], " "), err)
		}
		return string(out), fmt.Errorf("netsh %s: %w: %s", strings.Join(args[:min(4, len(args))], " "), err, msg)
	}
	return string(out), nil
}
