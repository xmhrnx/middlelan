package ui

import "github.com/xmhrnx/giode"

// pageHeader is a page's title, subtitle and actions.
func pageHeader(title, subtitle string, actions ...giode.Element) giode.Element {
	left := giode.Stack(giode.Styles{Gap: 4, FlexGrow: 1},
		giode.Text(title, styleTitle),
		giode.Text(subtitle, giode.Styles{Color: "theme.muted", FontSize: sizeBody}),
	)
	kids := append([]giode.Element{left}, actions...)
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Width: -1, Wrap: giode.WrapNoWrap, Padding: giode.Inset{Bottom: 4}}, kids...)
}
