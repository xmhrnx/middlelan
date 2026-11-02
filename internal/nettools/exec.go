package nettools

import (
	"context"
	"os/exec"
)

// runCmd runs an external command with a context and returns its combined
// output. A non-zero exit status is reported as an *exec.ExitError together
// with the output, since tools such as ping exit non-zero on packet loss.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd.CombinedOutput()
}
