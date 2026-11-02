package wifi

import (
	"strconv"
	"strings"
)

// nmcliFields is the field list passed to `nmcli -t -f ... dev wifi list`;
// parseNmcli expects this order.
const nmcliFields = "ACTIVE,SSID,BSSID,SIGNAL,FREQ,CHAN,SECURITY,RATE,DEVICE"

// splitNmcli splits one line of nmcli's terse (-t) output at unescaped
// colons and removes the backslash escapes nmcli adds before ':' and '\'.
func splitNmcli(line string) []string {
	var fields []string
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\' && i+1 < len(line):
			i++
			b.WriteByte(line[i])
		case c == ':':
			fields = append(fields, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	return append(fields, b.String())
}

// nmDBm estimates dBm from NetworkManager's signal percentage, inverting
// the mapping NetworkManager uses (-90 dBm is 0 %, -20 dBm is 100 %).
func nmDBm(q int) int {
	q = max(0, min(100, q))
	return -90 + q*70/100
}

// nmcliRow is one parsed line of `nmcli dev wifi list`.
type nmcliRow struct {
	Network
	RateMbps float64 // the access point's maximum rate
	Device   string
}

// parseNmcli parses `nmcli -t -f ` + nmcliFields + ` dev wifi list`.
func parseNmcli(out string) []nmcliRow {
	var rows []nmcliRow
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		f := splitNmcli(line)
		if len(f) < 8 {
			continue
		}
		var r nmcliRow
		r.Connected = f[0] == "yes" || f[0] == "*"
		r.SSID = f[1]
		r.BSSID = normMAC(f[2])
		r.Quality, _ = strconv.Atoi(strings.TrimSpace(f[3]))
		if r.Quality > 0 {
			r.SignalDBm = nmDBm(r.Quality)
		}
		r.FrequencyMHz, _ = strconv.Atoi(strings.Fields(f[4] + " 0")[0])
		r.Channel, _ = strconv.Atoi(strings.TrimSpace(f[5]))
		r.Security = securityFromText(f[6])
		r.RateMbps, _ = strconv.ParseFloat(strings.Fields(f[7] + " 0")[0], 64)
		if len(f) > 8 {
			r.Device = f[8]
		}
		fillRadio(&r.Channel, &r.FrequencyMHz, &r.Band)
		rows = append(rows, r)
	}
	return rows
}
