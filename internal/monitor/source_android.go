//go:build android

package monitor

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/xmhrnx/giode/android"
	"github.com/xmhrnx/middlelan/internal/vpn"
)

const netStatsClass = "com/middlelan/app/NetStats"

// androidSource combines what Android lets an app see: per-app totals from
// NetworkStatsManager (with Usage Access), the device's counters from
// TrafficStats, and, on Android 9 and earlier (or rooted devices), the
// socket tables.
type androidSource struct {
	linux  *linuxSource
	since  time.Time
	limits string
}

// NewSource returns the connection source for this platform.
func NewSource() Source {
	return &androidSource{linux: newLinuxSource(), since: startOfDay(time.Now())}
}

// UsageAccess reports whether the user granted Usage Access, which
// per-app traffic needs on Android.
func UsageAccess() bool {
	ok, err := android.CallBool(netStatsClass, "hasUsageAccess", "(Landroid/content/Context;)Z", android.Context())
	return err == nil && ok
}

// OpenUsageAccessSettings shows the system screen granting Usage Access.
func OpenUsageAccessSettings() error {
	return android.CallVoid(netStatsClass, "openUsageAccessSettings", "(Landroid/content/Context;)V", android.Context())
}

func (s *androidSource) Limits() string { return s.limits }

func (s *androidSource) Sample() (Sample, error) {
	var smp Sample
	var limits []string
	switch {
	case vpn.Default.Running():
		smp.Conns = vpnConns(vpn.Default.Flows())
	default:
		if socks, _, err := s.linux.sockets(); err == nil && len(socks) > 0 {
			smp.Conns = s.linux.owners.resolve(socks)
		} else {
			limits = append(limits, "Android hides other apps' connections. Turn on MiddleLAN's VPN in Security to see where each app connects.")
		}
	}
	if t, err := android.CallString(netStatsClass, "totals", "()Ljava/lang/String;"); err == nil {
		rx, tx, _ := strings.Cut(t, ",")
		smp.Iface.Rx, _ = strconv.ParseUint(rx, 10, 64)
		smp.Iface.Tx, _ = strconv.ParseUint(tx, 10, 64)
	} else if c, err := readIfaceCounters(); err == nil {
		smp.Iface = c
	}
	totals, err := s.appUsage()
	switch {
	case err == nil:
		smp.AppTotals = totals
		smp.PerApp = true
	case errors.Is(err, errUsageAccess):
		limits = append(limits, "Grant Usage Access to see how much data each app uses.")
	default:
		limits = append(limits, err.Error())
	}
	s.limits = strings.Join(limits, " ")
	return smp, nil
}

var errUsageAccess = errors.New("usage access not granted")

// vpnConns turns the VPN's flows into connections. Flows closed since the
// last sample are kept (as CLOSED) so their final bytes are counted.
func vpnConns(flows []vpn.Flow) []Conn {
	conns := make([]Conn, 0, len(flows))
	cutoff := time.Now().Add(-15 * time.Second)
	for _, f := range flows {
		if f.Closed && f.LastActive.Before(cutoff) {
			continue
		}
		state := "ESTABLISHED"
		if f.Closed {
			state = "CLOSED"
		}
		name := f.App
		if name == "" {
			name = f.Package
		}
		if f.Blocked {
			state = "CLOSED"
		}
		conns = append(conns, Conn{
			Key: "vpn:" + f.Key, Proto: f.Proto, LocalIP: f.LocalIP, LocalPort: f.LocalPort,
			RemoteIP: f.RemoteIP, RemotePort: f.RemotePort, State: state, UID: f.UID, Process: name,
			Sent: f.Sent, Recv: f.Recv, HasBytes: true,
		})
	}
	return conns
}

func (s *androidSource) appUsage() (map[string]ByteCount, error) {
	now := time.Now()
	if day := startOfDay(now); day.After(s.since) {
		s.since = day
	}
	out, err := android.CallString(netStatsClass, "appUsage", "(Landroid/content/Context;JJ)Ljava/lang/String;",
		android.Context(), s.since.UnixMilli(), now.UnixMilli()+60_000)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(out, "ERR:") {
		if out == "ERR:permission" {
			return nil, errUsageAccess
		}
		return nil, errors.New(strings.TrimPrefix(out, "ERR:"))
	}
	res := map[string]ByteCount{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		rx, _ := strconv.ParseUint(f[3], 10, 64)
		tx, _ := strconv.ParseUint(f[4], 10, 64)
		c := res[f[1]]
		c.Rx += rx
		c.Tx += tx
		res[f[1]] = c
	}
	return res, nil
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
