package wifi

import "strings"

// QualityFromDBm converts a signal strength in dBm to a 0-100 quality the
// way Windows does: -100 dBm or less is 0, -50 dBm or more is 100, and
// linear in between.
func QualityFromDBm(dbm int) int {
	switch {
	case dbm >= 0:
		return 0 // no reading
	case dbm <= -100:
		return 0
	case dbm >= -50:
		return 100
	}
	return 2 * (dbm + 100)
}

// DBmFromQuality is the inverse of QualityFromDBm: it estimates the signal
// strength in dBm from a 0-100 quality.
func DBmFromQuality(q int) int {
	q = max(0, min(100, q))
	return q/2 - 100
}

// ChannelFromFreq returns the IEEE 802.11 channel number of a centre
// frequency in MHz, or 0 when it isn't a Wi-Fi channel.
func ChannelFromFreq(mhz int) int {
	switch {
	case mhz == 2484:
		return 14
	case mhz >= 2412 && mhz <= 2472:
		return (mhz - 2407) / 5
	case mhz == 5935:
		return 2 // 6 GHz channel 2 sits below the regular grid
	case mhz >= 5955 && mhz <= 7115:
		return (mhz - 5950) / 5
	case mhz >= 5150 && mhz <= 5895:
		return (mhz - 5000) / 5
	case mhz >= 4910 && mhz <= 4980:
		return (mhz - 4000) / 5 // Japan's 4.9 GHz channels
	}
	return 0
}

// BandOf returns "2.4 GHz", "5 GHz" or "6 GHz" for a frequency in MHz, or
// "" when the frequency is outside those bands.
func BandOf(mhz int) string {
	switch {
	case mhz >= 2400 && mhz < 2500:
		return "2.4 GHz"
	case mhz >= 4900 && mhz < 5925:
		return "5 GHz"
	case mhz >= 5925 && mhz <= 7125:
		return "6 GHz"
	}
	return ""
}

// freqFromChannel returns the centre frequency of a channel in a band
// ("2.4 GHz", "5 GHz", "6 GHz"; anything else is guessed from the channel
// number, which is ambiguous between 5 and 6 GHz).
func freqFromChannel(ch int, band string) int {
	if ch <= 0 {
		return 0
	}
	switch normBand(band) {
	case "2.4 GHz":
		if ch == 14 {
			return 2484
		}
		if ch <= 13 {
			return 2407 + 5*ch
		}
	case "5 GHz":
		if ch >= 183 {
			return 4000 + 5*ch
		}
		return 5000 + 5*ch
	case "6 GHz":
		if ch == 2 {
			return 5935
		}
		return 5950 + 5*ch
	default:
		switch {
		case ch == 14:
			return 2484
		case ch <= 13:
			return 2407 + 5*ch
		case ch <= 177:
			return 5000 + 5*ch
		}
	}
	return 0
}

// normBand maps the band spellings tools use ("2.4 GHz", "2GHz", "5GHz",
// "6 GHz", "2.4") to BandOf's spelling, or "" when unknown.
func normBand(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
	s = strings.ReplaceAll(s, ",", ".")
	switch {
	case strings.HasPrefix(s, "2.4"), strings.HasPrefix(s, "2ghz"):
		return "2.4 GHz"
	case strings.HasPrefix(s, "5"):
		return "5 GHz"
	case strings.HasPrefix(s, "6"):
		return "6 GHz"
	}
	return ""
}

// fillRadio derives whichever of channel, frequency and band is missing
// from the others.
func fillRadio(ch, freq *int, band *string) {
	if *freq == 0 && *ch > 0 {
		*freq = freqFromChannel(*ch, *band)
	}
	if *ch == 0 && *freq > 0 {
		*ch = ChannelFromFreq(*freq)
	}
	if b := BandOf(*freq); b != "" {
		*band = b
	} else {
		*band = normBand(*band)
	}
}

// normMAC lower-cases a MAC address and uses colons, or returns "" for
// placeholders such as 00:00:00:00:00:00 and Android's 02:00:00:00:00:00.
func normMAC(s string) string {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "-", ":"))
	if len(s) != 17 || s == "00:00:00:00:00:00" || s == "02:00:00:00:00:00" || s == "ff:ff:ff:ff:ff:ff" {
		return ""
	}
	for i := 0; i < 17; i++ {
		c := s[i]
		if i%3 == 2 {
			if c != ':' {
				return ""
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return ""
		}
	}
	return s
}
