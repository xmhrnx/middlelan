package ui

import (
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// statTile is a key figure: an icon, a label, a big value and a note.
func statTile(icon, label, value, note string, accent giode.Color) giode.Element {
	return giode.Box(styleCard.With(giode.Styles{Gap: 10, Padding: giode.UniformInset(16)}),
		giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Wrap: giode.WrapNoWrap},
			iconBadge(icon, accent, 32),
			giode.Text(label, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FontWeight: giode.FontWeightMedium, TextOverflow: giode.TextOverflowEllipsis}),
		),
		giode.Text(value, giode.Styles{Color: "theme.text", FontSize: 24, FontWeight: giode.FontWeightBold, TextOverflow: giode.TextOverflowEllipsis}),
		giode.Text(note, giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
	)
}

// iconBadge is an icon on a soft tint of its color.
func iconBadge(icon string, c giode.Color, size int) giode.Element {
	return giode.Box(giode.Styles{
		Width: props.Width(size), Height: props.Height(size), BorderRadius: props.BorderRadius(size * 30 / 100),
		Background: giode.Mix(c, "theme.surface", 0.16), Align: giode.AlignCenter, Justify: giode.JustifyCenter,
	}, giode.Icon(icon, giode.Styles{Width: props.Width(size * 55 / 100), Color: c}))
}
