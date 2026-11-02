package ui

import "github.com/xmhrnx/giode"

// emptyState fills a space that has nothing to show yet.
func emptyState(icon, title, body string, action giode.Element) giode.Element {
	kids := []giode.Element{
		iconBadge(icon, "theme.primary", 52),
		giode.Text(title, giode.Styles{Color: "theme.text", FontSize: sizeHeading, FontWeight: giode.FontWeightSemiBold, TextAlign: giode.TextAlignCenter}),
		giode.Text(body, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, TextAlign: giode.TextAlignCenter, MaxWidth: 420}),
	}
	if action != nil {
		kids = append(kids, giode.Box(giode.Styles{Padding: giode.Inset{Top: 6}}, action))
	}
	return giode.Stack(giode.Styles{Align: giode.AlignCenter, Justify: giode.JustifyCenter, Gap: 10, Padding: giode.UniformInset(32), Width: -1}, kids...)
}
