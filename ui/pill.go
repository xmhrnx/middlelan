package ui

import (
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// pill is a small rounded label.
func pill(text string, fg giode.Color) giode.Element {
	return giode.Box(giode.Styles{
		Padding: giode.SymmetricInset(8, 3), BorderRadius: 99, Background: giode.Mix(fg, "theme.surface", 0.14),
	}, giode.Text(text, giode.Styles{Color: fg, FontSize: sizeTiny, FontWeight: giode.FontWeightSemiBold}))
}

// ratePair shows a download and an upload figure side by side.
func ratePair(rx, tx string, size float32) giode.Element {
	return giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
		giode.Row(giode.Styles{Gap: 4, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Icon("arrow-down", giode.Styles{Width: props.Width(size + 2), Color: colDown}),
			giode.Text(rx, giode.Styles{Color: "theme.text", FontSize: props.FontSize(size), FontWeight: giode.FontWeightMedium}),
		),
		giode.Row(giode.Styles{Gap: 4, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Icon("arrow-up", giode.Styles{Width: props.Width(size + 2), Color: colUp}),
			giode.Text(tx, giode.Styles{Color: "theme.text", FontSize: props.FontSize(size), FontWeight: giode.FontWeightMedium}),
		),
	)
}
