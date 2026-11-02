package ui

import (
	"fmt"
	"strconv"
	"time"
)

// formatBytes formats a byte count: "0 B", "812 KB", "1.4 GB".
func formatBytes(b uint64) string { return ltr(formatBytesRaw(b)) }

func formatBytesRaw(b uint64) string {
	const unit = 1000
	if b < unit {
		return strconv.FormatUint(b, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	v := float64(b) / float64(div)
	suffix := "KMGTP"[exp : exp+1]
	if v >= 100 {
		return fmt.Sprintf("%.0f %sB", v, suffix)
	}
	return fmt.Sprintf("%.1f %sB", v, suffix)
}

// formatRate formats bytes per second as bits per second, the unit
// networks are sold in: "0 bps", "12.4 Mbps".
func formatRate(bytesPerSec float64) string { return ltr(formatRateRaw(bytesPerSec)) }

func formatRateRaw(bytesPerSec float64) string {
	bits := bytesPerSec * 8
	switch {
	case bits < 1:
		return "0 bps"
	case bits < 1e3:
		return fmt.Sprintf("%.0f bps", bits)
	case bits < 1e6:
		return trimRate(bits/1e3) + " Kbps"
	case bits < 1e9:
		return trimRate(bits/1e6) + " Mbps"
	}
	return trimRate(bits/1e9) + " Gbps"
}

func trimRate(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// formatAgo formats how long ago t was: "now", "12s ago", "3h ago".
func formatAgo(t time.Time, now time.Time) string {
	if t.IsZero() {
		return tr("never")
	}
	d := now.Sub(t)
	switch {
	case d < 5*time.Second:
		return tr("now")
	case d < time.Minute:
		return tr("{n}s ago", "n", int(d.Seconds()))
	case d < time.Hour:
		return tr("{n}m ago", "n", int(d.Minutes()))
	case d < 48*time.Hour:
		return tr("{n}h ago", "n", int(d.Hours()))
	}
	return tr("{n}d ago", "n", int(d.Hours()/24))
}

// formatMs formats a duration in milliseconds: "12.3 ms".
func formatMs(d time.Duration) string { return ltr(formatMsRaw(d)) }

func formatMsRaw(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	if ms >= 100 {
		return fmt.Sprintf("%.0f ms", ms)
	}
	return fmt.Sprintf("%.1f ms", ms)
}

// plural returns "1 app" or "3 apps", translated. The message key is
// "{count} " + many ("{count} apps"); its locale entries carry the plural
// forms. Without an entry, one and many give the English forms.
func plural(n int, one, many string) string {
	s := trn("{count} "+many, n)
	if n == 1 && s == "1 "+many {
		return "1 " + one
	}
	return s
}

// endpointLabel joins an address and port, bracketing IPv6.
func endpointLabel(ip string, port int) string {
	for i := 0; i < len(ip); i++ {
		if ip[i] == ':' {
			return "[" + ip + "]:" + strconv.Itoa(port)
		}
	}
	return ip + ":" + strconv.Itoa(port)
}
