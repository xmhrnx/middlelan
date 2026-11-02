//go:build linux && !android

package wifi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// wirelessIfaces lists the network interfaces the kernel marks as
// wireless, falling back to `iw dev` when /sys isn't readable.
func wirelessIfaces(ctx context.Context) []string {
	var names []string
	entries, err := os.ReadDir("/sys/class/net")
	if err == nil {
		for _, e := range entries {
			for _, sub := range []string{"wireless", "phy80211"} {
				if _, err := os.Stat(filepath.Join("/sys/class/net", e.Name(), sub)); err == nil {
					names = append(names, e.Name())
					break
				}
			}
		}
	}
	if len(names) == 0 && haveCmd("iw") {
		if out, err := runCmd(ctx, "iw", "dev"); err == nil {
			names = parseIWDev(string(out))
		}
	}
	return names
}

// nmcliRows runs `nmcli dev wifi list`; rescan is "no" to use the cached
// list or "auto" to let NetworkManager refresh it when it is stale.
func nmcliRows(ctx context.Context, rescan string) ([]nmcliRow, error) {
	if !haveCmd("nmcli") {
		return nil, ErrUnsupported
	}
	out, err := runCmd(ctx, "nmcli", "-t", "-f", nmcliFields, "dev", "wifi", "list", "--rescan", rescan)
	if err != nil {
		return nil, err
	}
	return parseNmcli(string(out)), nil
}

// procSignal reads an interface's signal level from /proc/net/wireless.
func procSignal(iface string) int {
	b, err := os.ReadFile("/proc/net/wireless")
	if err != nil {
		return 0
	}
	return parseProcWireless(string(b), iface)
}

func current(ctx context.Context) (Link, error) {
	ifaces := wirelessIfaces(ctx)
	if len(ifaces) == 0 {
		return Link{}, ErrNotWiFi
	}
	if haveCmd("iw") {
		for _, ifc := range ifaces {
			out, err := runCmd(ctx, "iw", "dev", ifc, "link")
			if err != nil {
				continue
			}
			if l, ok := parseIWLink(string(out)); ok {
				if l.Interface == "" {
					l.Interface = ifc
				}
				l.Security = linuxSecurity(ctx, l.Interface, l.BSSID)
				return l, nil
			}
		}
	}
	if rows, err := nmcliRows(ctx, "no"); err == nil {
		for _, r := range rows {
			if !r.Connected {
				continue
			}
			l := Link{
				Interface: r.Device, SSID: r.SSID, BSSID: r.BSSID, Quality: r.Quality,
				SignalDBm: r.SignalDBm, Channel: r.Channel, FrequencyMHz: r.FrequencyMHz,
				Band: r.Band, Security: r.Security, Connected: true,
			}
			if l.Interface == "" {
				l.Interface = ifaces[0]
			}
			if dbm := procSignal(l.Interface); dbm != 0 {
				l.SignalDBm, l.Quality = dbm, QualityFromDBm(dbm)
			}
			finishLink(&l)
			return l, nil
		}
		return Link{Interface: ifaces[0]}, ErrNotWiFi
	}
	if haveCmd("iw") {
		return Link{Interface: ifaces[0]}, ErrNotWiFi // iw answered "Not connected." everywhere
	}
	// Neither tool is installed: the kernel still tells whether the link
	// is up and its signal, but not the network's name.
	for _, ifc := range ifaces {
		state, err := os.ReadFile(filepath.Join("/sys/class/net", ifc, "operstate"))
		if err != nil || strings.TrimSpace(string(state)) != "up" {
			continue
		}
		l := Link{Interface: ifc, SignalDBm: procSignal(ifc), Connected: true}
		finishLink(&l)
		return l, nil
	}
	return Link{Interface: ifaces[0]}, ErrNotWiFi
}

// linuxSecurity looks up the security of the access point the link uses,
// from NetworkManager's cached scan or iw's scan dump.
func linuxSecurity(ctx context.Context, iface, bssid string) Security {
	if bssid == "" {
		return SecUnknown
	}
	if rows, err := nmcliRows(ctx, "no"); err == nil {
		for _, r := range rows {
			if r.BSSID == bssid {
				return r.Security
			}
		}
	}
	if out, err := runCmd(ctx, "iw", "dev", iface, "scan", "dump"); err == nil {
		for _, n := range parseIWScan(string(out)) {
			if n.BSSID == bssid {
				return n.Security
			}
		}
	}
	return SecUnknown
}

func scan(ctx context.Context) ([]Network, error) {
	if rows, err := nmcliRows(ctx, "auto"); err == nil {
		nets := make([]Network, len(rows))
		for i, r := range rows {
			nets[i] = r.Network
		}
		return nets, nil
	}
	ifaces := wirelessIfaces(ctx)
	if len(ifaces) == 0 {
		return nil, ErrNotWiFi
	}
	if !haveCmd("iw") {
		return nil, ErrUnsupported
	}
	var nets []Network
	var lastErr error
	for _, ifc := range ifaces {
		out, err := runCmd(ctx, "iw", "dev", ifc, "scan", "dump")
		if err != nil {
			lastErr = err
			continue
		}
		nets = append(nets, parseIWScan(string(out))...)
	}
	if nets == nil && lastErr != nil {
		return nil, lastErr
	}
	return nets, nil
}
