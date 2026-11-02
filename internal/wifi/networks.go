package wifi

import (
	"cmp"
	"slices"
)

// finishNetworks fills in derived fields (channel, frequency, band, the
// missing one of dBm and quality) and sorts strongest first, then by SSID.
func finishNetworks(nets []Network) {
	for i := range nets {
		n := &nets[i]
		fillRadio(&n.Channel, &n.FrequencyMHz, &n.Band)
		switch {
		case n.SignalDBm == 0 && n.Quality > 0:
			n.SignalDBm = DBmFromQuality(n.Quality)
		case n.Quality == 0 && n.SignalDBm < 0:
			n.Quality = QualityFromDBm(n.SignalDBm)
		}
		if n.Security == "" {
			n.Security = SecUnknown
		}
	}
	slices.SortStableFunc(nets, func(a, b Network) int {
		if c := cmp.Compare(b.SignalDBm, a.SignalDBm); c != 0 {
			return c
		}
		return cmp.Compare(a.SSID, b.SSID)
	})
}

// finishLink fills in a Link's derived fields like finishNetworks.
func finishLink(l *Link) {
	fillRadio(&l.Channel, &l.FrequencyMHz, &l.Band)
	switch {
	case l.SignalDBm == 0 && l.Quality > 0:
		l.SignalDBm = DBmFromQuality(l.Quality)
	case l.Quality == 0 && l.SignalDBm < 0:
		l.Quality = QualityFromDBm(l.SignalDBm)
	}
	if l.Security == "" {
		l.Security = SecUnknown
	}
}
