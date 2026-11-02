package wifi

import (
	"context"
	"os"
	"os/exec"
)

// runCmd runs a tool with the C locale (so output is in English where the
// tool honours it) and returns its standard output. A non-zero exit
// status is returned as an *exec.ExitError together with the output.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	hideWindow(cmd)
	return cmd.Output()
}

// haveCmd reports whether a tool is on the PATH.
func haveCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
