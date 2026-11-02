package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/input"
)

// searchField is a text field styled for filtering lists.
func searchField(placeholder string) *input.Input {
	return textInputT(placeholder, styleField.With(giode.Styles{Padding: giode.SymmetricInset(12, 9)}))
}
