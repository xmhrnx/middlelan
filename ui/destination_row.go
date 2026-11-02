package ui

import (
	"sort"
	"strings"

	"github.com/mehran9675/giode"
)

// destinationRow is one owner apps talk to: name, place and apps.
func destinationRow(d *destination) giode.Element {
	apps := make([]string, 0, len(d.Apps))
	for a := range d.Apps {
		apps = append(apps, a)
	}
	sort.Strings(apps)
	if len(apps) > 3 {
		apps = append(apps[:3], "+"+itoa(len(apps)-3))
	}
	place := placeName(d.Place)
	if place == "" {
		place = tr("Locating…")
	}
	right := plural(d.Conns, "connection", "connections")
	if d.Bytes > 0 {
		right = formatBytes(d.Bytes)
	}
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 12, Padding: giode.SymmetricInset(4, 7), Width: -1, Wrap: giode.WrapNoWrap},
		countryBadge(d.CC, d.Scope),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Text(d.Owner, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(place+" · "+strings.Join(apps, ", "), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
		),
		giode.Text(right, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontWeight: giode.FontWeightMedium}),
	)
}
