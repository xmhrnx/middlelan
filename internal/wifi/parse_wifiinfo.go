package wifi

import (
	"errors"
	"strconv"
	"strings"
)

// This file parses the tab-separated strings returned by the Android Java
// helper com.middlelan.app.WifiInfo. It has no build tag so it is tested
// everywhere.

// androidUnknownSSID is what Android reports as the SSID when the app
// lacks location permission or location services are off.
const androidUnknownSSID = "<unknown ssid>"

// androidErr turns the helper's "ERR:" replies into errors: "ERR:off"
// (Wi-Fi disabled) and "ERR:nowifi" are ErrNotWiFi, "ERR:location" is
// ErrNeedsLocation.
func androidErr(s string) error {
	reason, ok := strings.CutPrefix(s, "ERR:")
	if !ok {
		return nil
	}
	switch reason = strings.TrimSpace(reason); reason {
	case "off", "nowifi":
		return ErrNotWiFi
	case "location":
		return ErrNeedsLocation
	case "permission":
		return errors.New("wifi: the app lacks the ACCESS_WIFI_STATE permission")
	}
	return errors.New("wifi: " + reason)
}

// parseAndroidCurrent parses WifiInfo.current: "connected\tssid\tbssid\t
// rssi\tfrequency\tlinkSpeed\ttxSpeed\trxSpeed\tsecurityType\tcapabilities".
func parseAndroidCurrent(s string) (Link, error) {
	if err := androidErr(s); err != nil {
		return Link{}, err
	}
	f := strings.Split(strings.TrimRight(s, "\r\n"), "\t")
	if len(f) < 10 {
		return Link{}, errors.New("wifi: unexpected reply from the Android helper")
	}
	if f[0] != "1" {
		return Link{}, ErrNotWiFi
	}
	atoi := func(v string) int { n, _ := strconv.Atoi(strings.TrimSpace(v)); return n }
	l := Link{Connected: true, BSSID: normMAC(f[2])}
	ssid := f[1]
	if len(ssid) >= 2 && strings.HasPrefix(ssid, `"`) && strings.HasSuffix(ssid, `"`) {
		ssid = ssid[1 : len(ssid)-1]
	}
	needLoc := ssid == androidUnknownSSID
	if !needLoc {
		l.SSID = ssid
	}
	if rssi := atoi(f[3]); rssi < 0 && rssi > -127 {
		l.SignalDBm = rssi
	}
	l.FrequencyMHz = atoi(f[4])
	link := atoi(f[5])
	if tx := atoi(f[6]); tx > 0 {
		l.TxRateMbps = float64(tx)
	} else if link > 0 {
		l.TxRateMbps = float64(link)
	}
	if rx := atoi(f[7]); rx > 0 {
		l.RxRateMbps = float64(rx)
	}
	l.Security = SecUnknown
	if t := strings.TrimSpace(f[8]); t != "" {
		l.Security = securityFromAndroidType(atoi(t))
	}
	if l.Security == SecUnknown && f[9] != "" {
		l.Security = securityFromAndroidCaps(f[9])
	}
	finishLink(&l)
	if needLoc {
		return l, ErrNeedsLocation
	}
	return l, nil
}

// parseAndroidScan parses WifiInfo.scan: one network per line,
// "ssid\tbssid\tlevel\tfrequency\tcapabilities".
func parseAndroidScan(s string) ([]Network, error) {
	if err := androidErr(s); err != nil {
		return nil, err
	}
	var nets []Network
	for _, line := range strings.Split(s, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 5 {
			continue
		}
		n := Network{SSID: f[0], BSSID: normMAC(f[1]), Security: securityFromAndroidCaps(f[4])}
		n.SignalDBm, _ = strconv.Atoi(f[2])
		n.FrequencyMHz, _ = strconv.Atoi(f[3])
		fillRadio(&n.Channel, &n.FrequencyMHz, &n.Band)
		if n.SignalDBm < 0 {
			n.Quality = QualityFromDBm(n.SignalDBm)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// markConnected sets Connected on the networks whose BSSID (or, when the
// BSSID is unknown, SSID) matches the current link.
func markConnected(nets []Network, l Link) {
	for i := range nets {
		n := &nets[i]
		switch {
		case l.BSSID != "" && n.BSSID != "":
			n.Connected = n.Connected || n.BSSID == l.BSSID
		case l.SSID != "" && n.BSSID == "" && n.SSID == l.SSID && n.Channel == l.Channel:
			n.Connected = true
		}
	}
}
