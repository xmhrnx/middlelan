//go:build !windows

package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// cmdTimeout bounds one run of nft or pfctl.
const cmdTimeout = 30 * time.Second

// runTool runs a command with stdin and returns its combined output. A
// failure includes the tool's message.
func runTool(stdin string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			return out.String(), fmt.Errorf("%s: %w", name, err)
		}
		return out.String(), fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return out.String(), nil
}

// findTool returns the path of a system tool, looking in the sbin
// directories that a desktop session's PATH often lacks.
func findTool(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range []string{"/usr/sbin", "/sbin", "/usr/local/sbin"} {
		if p, err := exec.LookPath(dir + "/" + name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s isn't installed", name)
}
