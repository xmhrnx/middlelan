package ui

import (
	"gioui.org/layout"
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// cardElement is a card that a grid can stretch to the height of the
// tallest card in its row.
type cardElement struct {
	st   giode.Styles
	kids []giode.Element
}

func (c cardElement) Layout(gtx layout.Context) layout.Dimensions {
	return giode.Box(c.st, c.kids...).Layout(gtx)
}

// withHeight is the card at a fixed height in dp; its content keeps its
// place at the top.
func (c cardElement) withHeight(h int) giode.Element {
	return giode.Box(c.st.With(giode.Styles{Height: props.Height(h)}), c.kids...)
}

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
	return cardElement{st: styleCard, kids: kids}
}

// cardStyled is card with extra styles merged over the default.
func cardStyled(st giode.Styles, body ...giode.Element) giode.Element {
	return cardElement{st: styleCard.With(st), kids: body}
}
