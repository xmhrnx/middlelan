//go:build darwin && !ios

package wifi

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// airportPath is the legacy command-line tool, removed in macOS 14.4.
const airportPath = "/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"

func systemProfiler(ctx context.Context) (l Link, ok bool, nets []Network, redacted bool, err error) {
	out, err := runCmd(ctx, "system_profiler", "SPAirPortDataType", "-json")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			err = ErrUnsupported
		}
		return Link{}, false, nil, false, err
	}
	return parseSystemProfiler(out)
}

func haveAirport() bool {
	_, err := os.Stat(airportPath)
	return err == nil
}

func current(ctx context.Context) (Link, error) {
	l, ok, _, redacted, err := systemProfiler(ctx)
	if err != nil {
		return Link{}, err
	}
	if !ok {
		return l, ErrNotWiFi
	}
	if haveAirport() {
		if out, err := runCmd(ctx, airportPath, "-I"); err == nil {
			if a, aok := parseAirportInfo(string(out)); aok {
				if l.SSID == "" && a.SSID != "" {
					l.SSID, redacted = a.SSID, false
				}
				if l.BSSID == "" {
					l.BSSID = a.BSSID
				}
				if a.SignalDBm < 0 {
					l.SignalDBm, l.Quality = a.SignalDBm, a.Quality
				}
				if a.TxRateMbps > 0 {
					l.TxRateMbps = a.TxRateMbps
				}
			}
		}
	}
	if redacted && l.SSID == "" {
		return l, ErrNeedsLocation
	}
	return l, nil
}

func scan(ctx context.Context) ([]Network, error) {
	if haveAirport() {
		if out, err := runCmd(ctx, airportPath, "-s"); err == nil {
			if nets := parseAirportScan(string(out)); len(nets) > 0 {
				if l, err := current(ctx); err == nil || errors.Is(err, ErrNeedsLocation) {
					markConnected(nets, l)
				}
				return nets, nil
			}
		}
	}
	_, _, nets, _, err := systemProfiler(ctx)
	if err != nil {
		return nil, err
	}
	return nets, nil
}
