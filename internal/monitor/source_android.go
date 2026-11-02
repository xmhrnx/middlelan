//go:build android

package monitor

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/mehran9675/giode/android"
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
	if socks, _, err := s.linux.sockets(); err == nil && len(socks) > 0 {
		smp.Conns = s.linux.owners.resolve(socks)
	} else {
		limits = append(limits, "Android hides other apps' connections; destinations are shown for this device's own traffic only.")
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
