//go:build windows

package wifi

import (
	"context"
	"errors"
	"os/exec"
)

// netsh runs a `netsh wlan` command and maps its refusals to this
// package's errors.
func netsh(ctx context.Context, args ...string) (string, error) {
	out, err := runCmd(ctx, "netsh", append([]string{"wlan"}, args...)...)
	s := string(out)
	switch {
	case netshNeedsLocation(s):
		return "", ErrNeedsLocation
	case netshNoWiFi(s):
		return "", ErrNotWiFi
	case errors.Is(err, exec.ErrNotFound):
		return "", ErrUnsupported
	case err != nil && len(out) == 0:
		return "", err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	return s, nil
}

func current(ctx context.Context) (Link, error) {
	out, err := netsh(ctx, "show", "interfaces")
	if err != nil {
		return Link{}, err
	}
	l, ok := parseNetshInterfaces(out)
	if !ok {
		return l, ErrNotWiFi
	}
	return l, nil
}

func scan(ctx context.Context) ([]Network, error) {
	out, err := netsh(ctx, "show", "networks", "mode=bssid")
	if err != nil {
		return nil, err
	}
	nets := parseNetshNetworks(out)
	if l, err := current(ctx); err == nil {
		markConnected(nets, l)
	}
	return nets, nil
}
