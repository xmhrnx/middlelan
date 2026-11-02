package ui

import (
	"strconv"

	"github.com/mehran9675/giode"
	"github.com/xmhrnx/middlelan/internal/geo"
)

// countryBadge shows a country code (or the kind of address) in a tile;
// the bundled fonts have no flag emoji, so codes stand in for flags.
func countryBadge(cc string, scope geo.Scope) giode.Element {
	label, c := cc, giode.Color("theme.info")
	switch {
	case scope == geo.ScopePrivate || scope == geo.ScopeLinkLocal:
		label, c = "LAN", "theme.success"
	case scope == geo.ScopeLoopback:
		label, c = "PC", "theme.muted"
	case scope == geo.ScopeMulticast:
		label, c = "ALL", "theme.warning"
	case label == "":
		label, c = "··", "theme.muted"
	}
	return giode.Box(giode.Styles{
		Width: 36, Height: 28, BorderRadius: 8, Background: giode.Mix(c, "theme.surface", 0.15),
		Align: giode.AlignCenter, Justify: giode.JustifyCenter,
	}, giode.Text(label, giode.Styles{Color: c, FontSize: sizeTiny, FontWeight: giode.FontWeightBold}))
}

func itoa(n int) string { return strconv.Itoa(n) }
