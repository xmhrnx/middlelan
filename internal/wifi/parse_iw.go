package wifi

import (
	"strconv"
	"strings"
)

// unescapeIW decodes the \xNN escapes iw uses for non-printable SSID bytes.
func unescapeIW(s string) string {
	if !strings.Contains(s, `\x`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x' {
			if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// iwNum parses the leading number of values such as "5180", "5180.0",
// "-52 dBm" or "866.7 MBit/s".
func iwNum(v string) float64 {
	f, err := strconv.ParseFloat(strings.Fields(v + " 0")[0], 64)
	if err != nil {
		return 0
	}
	return f
}

// parseIWDev parses `iw dev` and returns the names of managed (station)
// interfaces.
func parseIWDev(out string) []string {
	var names []string
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "Interface "):
			cur = strings.TrimSpace(strings.TrimPrefix(t, "Interface "))
		case strings.HasPrefix(t, "type ") && cur != "":
			if strings.TrimSpace(strings.TrimPrefix(t, "type ")) == "managed" {
				names = append(names, cur)
			}
			cur = ""
		}
	}
	return names
}

// parseIWLink parses `iw dev <iface> link`. ok is false when the output
// says "Not connected.".
func parseIWLink(out string) (l Link, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(t, "Connected to "); found {
			f := strings.Fields(rest)
			if len(f) > 0 {
				l.BSSID = normMAC(f[0])
			}
			if i := strings.Index(rest, "(on "); i >= 0 {
				l.Interface = strings.TrimSuffix(strings.Fields(rest[i+4:] + " ")[0], ")")
			}
			ok = true
			continue
		}
		k, v, found := strings.Cut(t, ":")
		if !found {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "SSID":
			l.SSID = unescapeIW(v)
		case "freq":
			l.FrequencyMHz = int(iwNum(v))
		case "signal":
			l.SignalDBm = int(iwNum(v))
		case "rx bitrate":
			l.RxRateMbps = iwNum(v)
		case "tx bitrate":
			l.TxRateMbps = iwNum(v)
		}
	}
	if !ok {
		return Link{}, false
	}
	l.Connected = true
	finishLink(&l)
	return l, true
}

// parseIWScan parses `iw dev <iface> scan dump` (or `scan`).
func parseIWScan(out string) []Network {
	var nets []Network
	var cur *Network
	var f secFlags
	var privacy, sawRSN, sawWPA bool
	section := ""
	flush := func() {
		if cur == nil {
			return
		}
		switch {
		case sawRSN || sawWPA:
			cur.Security = f.security()
			if cur.Security == SecUnknown {
				cur.Security = SecWPA2 // RSN with an unrecognised key management suite
			}
		case privacy:
			cur.Security = SecWEP
		default:
			cur.Security = SecOpen
		}
		nets = append(nets, *cur)
		cur = nil
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "BSS "); ok {
			flush()
			cur = &Network{}
			f, privacy, sawRSN, sawWPA, section = secFlags{}, false, false, false, ""
			mac := rest
			if i := strings.IndexAny(mac, "( "); i >= 0 {
				mac = mac[:i]
			}
			cur.BSSID = normMAC(mac)
			cur.Connected = strings.Contains(rest, "-- associated")
			continue
		}
		if cur == nil {
			continue
		}
		// Top-level fields are indented by one tab; RSN/WPA details by two.
		top := strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t")
		t := strings.TrimSpace(line)
		if top {
			k, v, _ := strings.Cut(t, ":")
			v = strings.TrimSpace(v)
			section = ""
			switch k {
			case "freq":
				cur.FrequencyMHz = int(iwNum(v))
			case "signal":
				cur.SignalDBm = int(iwNum(v))
			case "SSID":
				cur.SSID = unescapeIW(v)
			case "capability":
				privacy = strings.Contains(v, "Privacy")
			case "DS Parameter set":
				if c, ok := strings.CutPrefix(v, "channel "); ok {
					cur.Channel, _ = strconv.Atoi(strings.TrimSpace(c))
				}
			case "RSN":
				section, sawRSN = "RSN", true
			case "WPA":
				section, sawWPA = "WPA", true
				f.wpa1 = true
			}
			// The first sub-item may share the section's line: "RSN:\t * Version: 1".
			if section == "" {
				continue
			}
			t = strings.TrimSpace(v)
		}
		t = strings.TrimSpace(strings.TrimPrefix(t, "*"))
		if suites, ok := strings.CutPrefix(t, "Authentication suites:"); ok {
			iwSuites(&f, section, suites)
		}
	}
	flush()
	return nets
}

// iwSuites records the key management suites of an RSN or WPA element.
func iwSuites(f *secFlags, section, suites string) {
	s := strings.ToUpper(suites)
	if strings.Contains(s, "802.1X") || strings.Contains(s, "SUITE-B") || strings.Contains(s, "FILS") {
		f.enterprise = true
	}
	if section != "RSN" {
		return
	}
	for _, w := range strings.Fields(s) {
		w = strings.TrimPrefix(w, "FT/")
		switch w {
		case "PSK", "PSK/SHA-256":
			f.wpa2 = true
		case "SAE", "SAE-EXT-KEY", "OWE":
			f.wpa3 = true
		}
	}
}

// parseProcWireless returns the signal level in dBm of an interface from
// /proc/net/wireless, or 0 when it isn't listed.
func parseProcWireless(out, iface string) int {
	for _, line := range strings.Split(out, "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || name != iface {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 3 {
			return 0
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(f[2], "."), 64)
		if err != nil {
			return 0
		}
		if v > 0 { // some drivers report the level as an unsigned byte
			v -= 256
		}
		if v >= 0 || v < -120 {
			return 0
		}
		return int(v)
	}
	return 0
}
