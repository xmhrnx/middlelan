package wifi

import (
	"regexp"
	"strconv"
	"strings"
)

// netsh's labels are translated, so the parsers below recognise fields by
// their values or by tokens that stay the same in every language (SSID,
// BSSID, %, Mbps, GHz) and fall back to English and common translations
// for the rest.

var (
	netshKV      = regexp.MustCompile(`^(\s*)([^:]*?)\s*:\s?(.*)$`)
	netshSSIDHdr = regexp.MustCompile(`^SSID\s+\d+\s*$`)
	netshBSSHdr  = regexp.MustCompile(`^BSSID\s+\d+\s*$`)
	netshPercent = regexp.MustCompile(`^(\d{1,3})\s*%$`)
	netshBandVal = regexp.MustCompile(`(?i)^\d+([.,]\d+)?\s*GHz$`)
	netshGUID    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-`)
	netshAuthVal = regexp.MustCompile(`(?i)^(open|shared|wpa\S*|owe\S*|802\.1x)$|personal|enterprise`)
	netshCipher  = map[string]bool{"NONE": true, "WEP": true, "TKIP": true, "CCMP": true, "GCMP": true, "CCMP-256": true, "GCMP-256": true, "BIP": true}
)

var netshChannelKeys = map[string]bool{
	"channel": true, "kanal": true, "canal": true, "canale": true, "kanaal": true,
	"канал": true, "kanał": true, "チャネル": true, "频道": true, "通道": true, "채널": true,
}

var netshAuthKeys = map[string]bool{
	"authentication": true, "authentifizierung": true, "authentification": true,
	"autenticación": true, "autenticazione": true, "autenticação": true, "authenticatie": true,
}

// netshLine splits a "  Key   : value" line. It returns ok=false for
// lines without a colon.
func netshLine(line string) (indent int, key, val string, ok bool) {
	m := netshKV.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil {
		return 0, "", "", false
	}
	return len(m[1]), strings.TrimSpace(m[2]), strings.TrimSpace(m[3]), true
}

func netshIsChannelKey(k string) bool { return netshChannelKeys[strings.ToLower(k)] }

func netshIsAuthKey(k string, v string) bool {
	return netshAuthKeys[strings.ToLower(k)] || netshAuthVal.MatchString(v)
}

func netshIsCipher(k, v string) bool {
	lk := strings.ToLower(k)
	return lk == "cipher" || lk == "encryption" || netshCipher[strings.ToUpper(v)]
}

// netshMbps parses a rate such as "866.7" or "866,7".
func netshMbps(v string) float64 {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.Fields(v + " 0")[0], ",", "."), 64)
	if err != nil {
		return 0
	}
	return f
}

// netshNeedsLocation reports whether netsh refused because Windows 11
// (24H2 and later) requires location permission for Wi-Fi details.
func netshNeedsLocation(out string) bool {
	l := strings.ToLower(out)
	return strings.Contains(l, "ms-settings:privacy-location") || strings.Contains(l, "location permission")
}

// netshNoWiFi reports whether netsh said there is no wireless interface
// or the WLAN AutoConfig service isn't running.
func netshNoWiFi(out string) bool {
	l := strings.ToLower(out)
	return strings.Contains(l, "no wireless interface") || strings.Contains(l, "wlansvc")
}

// parseNetshInterfaces parses `netsh wlan show interfaces` and returns
// the first interface connected to a network (it has an SSID and BSSID).
// ok is false when no interface is connected; the first interface's name
// is still returned then.
func parseNetshInterfaces(out string) (l Link, ok bool) {
	for _, para := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n\n") {
		var c Link
		var auth, cipher string
		mbps := 0
		first := true
		for _, line := range strings.Split(para, "\n") {
			_, k, v, isKV := netshLine(line)
			if !isKV || k == "" {
				continue
			}
			if first && v != "" {
				c.Interface = v
				first = false
			}
			lk := strings.ToLower(k)
			switch {
			case k == "SSID":
				c.SSID = v
			case strings.Contains(k, "BSSID"):
				c.BSSID = normMAC(v)
			case lk == "rssi":
				c.SignalDBm, _ = strconv.Atoi(v)
			case netshPercent.MatchString(v):
				q, _ := strconv.Atoi(netshPercent.FindStringSubmatch(v)[1])
				c.Quality = min(q, 100)
			case strings.Contains(lk, "mbps"):
				r := netshMbps(v)
				switch {
				case strings.Contains(lk, "receive"):
					c.RxRateMbps = r
				case strings.Contains(lk, "transmit"):
					c.TxRateMbps = r
				case mbps == 0:
					c.RxRateMbps = r
				default:
					c.TxRateMbps = r
				}
				mbps++
			case netshBandVal.MatchString(v):
				c.Band = normBand(v)
			case netshIsChannelKey(k):
				c.Channel, _ = strconv.Atoi(v)
			case netshGUID.MatchString(v), strings.Contains(lk, "physical"), strings.Contains(lk, "profile"):
			case netshIsCipher(k, v):
				cipher = v
			case netshIsAuthKey(k, v):
				auth = v
			}
		}
		if c.SSID == "" && c.BSSID == "" {
			if !ok && l.Interface == "" && c.Interface != "" && strings.Contains(para, "GUID") {
				l.Interface = c.Interface
			}
			continue
		}
		c.Connected = true
		if auth != "" || cipher != "" {
			c.Security = netshSecurity(auth, cipher)
		}
		finishLink(&c)
		return c, true
	}
	return l, false
}

// netshSecurity combines netsh's authentication and cipher values.
func netshSecurity(auth, cipher string) Security {
	f := flagsFromText(auth + " " + cipher)
	if strings.EqualFold(auth, "open") && strings.EqualFold(cipher, "wep") {
		return SecWEP
	}
	if s := f.security(); s != SecUnknown {
		return s
	}
	if strings.EqualFold(cipher, "none") {
		return SecOpen
	}
	return SecUnknown
}

// parseNetshNetworks parses `netsh wlan show networks mode=bssid`, one
// Network per BSSID.
func parseNetshNetworks(out string) []Network {
	var nets []Network
	var ssid, auth, cipher string
	var cur *Network
	flush := func() {
		if cur != nil {
			nets = append(nets, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		_, k, v, isKV := netshLine(line)
		if !isKV {
			continue
		}
		switch {
		case netshSSIDHdr.MatchString(k):
			flush()
			ssid, auth, cipher = v, "", ""
		case netshBSSHdr.MatchString(k):
			flush()
			cur = &Network{SSID: ssid, BSSID: normMAC(v), Security: netshSecurity(auth, cipher)}
		case cur == nil:
			// Network-level fields come before the first BSSID.
			switch {
			case netshIsCipher(k, v):
				cipher = v
			case netshIsAuthKey(k, v):
				auth = v
			}
		case netshPercent.MatchString(v):
			q, _ := strconv.Atoi(netshPercent.FindStringSubmatch(v)[1])
			cur.Quality = min(q, 100)
		case strings.EqualFold(k, "rssi"):
			cur.SignalDBm, _ = strconv.Atoi(v)
		case netshBandVal.MatchString(v):
			cur.Band = normBand(v)
		case netshIsChannelKey(k):
			cur.Channel, _ = strconv.Atoi(v)
		}
	}
	flush()
	return nets
}
