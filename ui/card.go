package ui

import "github.com/mehran9675/giode"

// card is a rounded surface with an optional title row.
func card(title string, action giode.Element, body ...giode.Element) giode.Element {
	kids := make([]giode.Element, 0, len(body)+1)
	if title != "" || action != nil {
		head := []giode.Element{giode.Text(title, giode.Styles{Color: "theme.text", FontSize: 15, FontWeight: giode.FontWeightSemiBold, FlexGrow: 1})}
		if action != nil {
			head = append(head, action)
		}
		kids = append(kids, giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 8, Wrap: giode.WrapNoWrap}, head...))
	}
	kids = append(kids, body...)
	return giode.Box(styleCard, kids...)
}

// cardStyled is card with extra styles merged over the default.
func cardStyled(st giode.Styles, body ...giode.Element) giode.Element {
	return giode.Box(styleCard.With(st), body...)
}
