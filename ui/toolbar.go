package ui

import "github.com/mehran9675/giode"

// toolbar puts a search field first and controls after it, wrapping on
// narrow screens.
func toolbar(search giode.Element, controls ...giode.Element) giode.Element {
	kids := []giode.Element{giode.Box(giode.Styles{FlexGrow: 1, MinWidth: 220, Width: -1}, search)}
	kids = append(kids, controls...)
	return giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Width: -1}, kids...)
}

// pageBodyFill is pageBody for pages whose last child fills the height
// (a list), rather than scrolling as a whole.
func pageBodyFill(kids ...giode.Element) giode.Element {
	pad := giode.UniformInset(28)
	if giode.Compact() {
		pad = giode.UniformInset(14)
	}
	return giode.Stack(giode.Styles{Gap: 16, Padding: pad, Width: -1, Height: -1, MaxWidth: 1280}, compactKids(kids)...)
}
