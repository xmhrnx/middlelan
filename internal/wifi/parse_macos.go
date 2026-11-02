package wifi

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// macRedacted is what system_profiler shows instead of a network name when
// the calling app lacks location permission.
const macRedacted = "<redacted>"

type spNetwork struct {
	Name     string          `json:"_name"`
	BSSID    string          `json:"spairport_network_bssid"`
	Channel  json.RawMessage `json:"spairport_network_channel"`
	Rate     json.RawMessage `json:"spairport_network_rate"`
	Security string          `json:"spairport_security_mode"`
	Signal   string          `json:"spairport_signal_noise"`
}

type spInterface struct {
	Name    string      `json:"_name"`
	Status  string      `json:"spairport_status_information"`
	Current *spNetwork  `json:"spairport_current_network_information"`
	Others  []spNetwork `json:"spairport_airport_other_local_wireless_networks"`
	Local   []spNetwork `json:"spairport_airport_local_wireless_networks"`
}

type spReport struct {
	Items []struct {
		Interfaces []spInterface `json:"spairport_airport_interfaces"`
	} `json:"SPAirPortDataType"`
}

var spChannel = regexp.MustCompile(`^\s*(\d+)\s*(?:\(([^,)]*))?`)

// spChannelBand parses "149 (5GHz, 80MHz)" or a bare number.
func spChannelBand(raw json.RawMessage) (ch int, band string) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return 0, ""
		}
		return int(n), ""
	}
	m := spChannel.FindStringSubmatch(s)
	if m == nil {
		return 0, ""
	}
	ch, _ = strconv.Atoi(m[1])
	return ch, normBand(m[2])
}

// spNumber reads a JSON number or numeric string.
func spNumber(raw json.RawMessage) float64 {
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return iwNum(s)
	}
	return 0
}

// spSignal parses "-52 dBm / -93 dBm" into the signal level.
func spSignal(s string) int {
	return int(iwNum(strings.TrimSpace(s)))
}

func (n spNetwork) network() (Network, bool) {
	out := Network{SSID: n.Name, BSSID: normMAC(n.BSSID), SignalDBm: spSignal(n.Signal)}
	redacted := n.Name == macRedacted
	if redacted {
		out.SSID = ""
	}
	out.Channel, out.Band = spChannelBand(n.Channel)
	if n.Security != "" {
		out.Security = securityFromText(n.Security)
	}
	fillRadio(&out.Channel, &out.FrequencyMHz, &out.Band)
	if out.SignalDBm < 0 {
		out.Quality = QualityFromDBm(out.SignalDBm)
	}
	return out, redacted
}

// parseSystemProfiler parses `system_profiler SPAirPortDataType -json`. It
// returns the current link (ok false when not connected), the other
// networks the system last saw, and whether names were redacted.
func parseSystemProfiler(data []byte) (l Link, ok bool, nets []Network, redacted bool, err error) {
	var r spReport
	if err := json.Unmarshal(data, &r); err != nil {
		return Link{}, false, nil, false, err
	}
	for _, item := range r.Items {
		for _, ifc := range item.Interfaces {
			if ifc.Current == nil && len(ifc.Others) == 0 && len(ifc.Local) == 0 {
				continue
			}
			if ifc.Current != nil && !ok {
				n, red := ifc.Current.network()
				redacted = redacted || red
				l = Link{
					Interface: ifc.Name, SSID: n.SSID, BSSID: n.BSSID,
					SignalDBm: n.SignalDBm, Channel: n.Channel, Band: n.Band,
					FrequencyMHz: n.FrequencyMHz, Security: n.Security, Connected: true,
					TxRateMbps: spNumber(ifc.Current.Rate),
				}
				finishLink(&l)
				ok = true
				n.Connected = true
				nets = append(nets, n)
			} else if l.Interface == "" {
				l.Interface = ifc.Name
			}
			for _, o := range append(ifc.Others, ifc.Local...) {
				n, red := o.network()
				redacted = redacted || red
				nets = append(nets, n)
			}
		}
	}
	return l, ok, nets, redacted, nil
}

// parseAirportInfo parses the legacy `airport -I` output. ok is false when
// it printed nothing useful (on macOS 14.4 and later it only prints a
// deprecation notice).
func parseAirportInfo(out string) (l Link, ok bool) {
	var auth string
	for _, line := range strings.Split(out, "\n") {
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "agrCtlRSSI":
			l.SignalDBm, _ = strconv.Atoi(v)
		case "lastTxRate":
			l.TxRateMbps = iwNum(v)
		case "BSSID":
			l.BSSID = normMAC(airportMAC(v))
		case "SSID":
			l.SSID = v
			ok = true
		case "link auth":
			auth = v
		case "channel":
			c, _, _ := strings.Cut(v, ",")
			l.Channel, _ = strconv.Atoi(c)
		case "state":
			l.Connected = v == "running"
		}
	}
	if !ok {
		return Link{}, false
	}
	if auth != "" {
		l.Security = airportAuth(auth)
	}
	finishLink(&l)
	return l, true
}

// airportAuth maps airport's "link auth" values (wpa2-psk, wpa3-sae, none…).
func airportAuth(a string) Security {
	a = strings.ToLower(a)
	switch {
	case a == "none" || a == "open":
		return SecOpen
	case strings.Contains(a, "8021x") || strings.Contains(a, "802.1x") || strings.Contains(a, "eap"):
		return SecEnterprise
	case strings.Contains(a, "sae") || strings.Contains(a, "wpa3"):
		return SecWPA3
	case strings.Contains(a, "wpa2"):
		return SecWPA2
	case strings.Contains(a, "wpa"):
		return SecWPA
	case strings.Contains(a, "wep"), a == "shared":
		return SecWEP
	}
	return SecUnknown
}

// airportMAC pads the octets airport prints without leading zeros
// ("0:1b:2c:3:4:5").
func airportMAC(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return s
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}

var airportScanRow = regexp.MustCompile(`^\s*(.*?)\s+([0-9a-fA-F]{1,2}(?::[0-9a-fA-F]{1,2}){5})\s+(-?\d+)\s+(\d+)(?:,\S+)?\s+\S+\s+\S+\s+(.*)$`)

// parseAirportScan parses the legacy `airport -s` table.
func parseAirportScan(out string) []Network {
	var nets []Network
	for _, line := range strings.Split(out, "\n") {
		m := airportScanRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n := Network{SSID: m[1], BSSID: normMAC(airportMAC(m[2])), Security: securityFromText(m[5])}
		n.SignalDBm, _ = strconv.Atoi(m[3])
		n.Channel, _ = strconv.Atoi(m[4])
		fillRadio(&n.Channel, &n.FrequencyMHz, &n.Band)
		n.Quality = QualityFromDBm(n.SignalDBm)
		nets = append(nets, n)
	}
	return nets
}
