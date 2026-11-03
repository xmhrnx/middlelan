package ui

import "github.com/xmhrnx/giode"

// statusDot is a status dot with a soft halo. It's deliberately static:
// an animation would redraw the window continuously, and MiddleMan must
// stay light on resources.
func statusDot(c giode.Color, live bool) giode.Element {
	return giode.Canvas(giode.Styles{Width: 14, Height: 14}, func(p *giode.Painter) {
		if live {
			p.Circle(7, 7, 7, giode.Mix(c, "theme.surface", 0.3))
		}
		p.Circle(7, 7, 4, c)
	})
}
