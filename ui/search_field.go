package ui

import (
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
)

// searchField is a text field styled for filtering lists.
func searchField(placeholder string) *input.Input {
	return giode.Input.Text(placeholder, styleField.With(giode.Styles{Padding: giode.SymmetricInset(12, 9)}))
}
