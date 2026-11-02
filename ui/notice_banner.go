package ui

import "github.com/mehran9675/giode"

// noticeBanner is an inline message with an optional action.
func noticeBanner(icon, text string, tone giode.Color, action giode.Element) giode.Element {
	kids := []giode.Element{
		giode.Icon(icon, giode.Styles{Width: 18, Color: tone}),
		giode.Text(text, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1}),
	}
	if action != nil {
		kids = append(kids, action)
	}
	return giode.Row(giode.Styles{
		Align: giode.AlignCenter, Gap: 12, Padding: giode.SymmetricInset(14, 12), Width: -1, Wrap: giode.WrapNoWrap,
		Background: giode.Mix(tone, "theme.surface", 0.1), BorderRadius: 12, BorderWidth: 1, BorderColor: giode.Mix(tone, "theme.surface", 0.35),
	}, kids...)
}
