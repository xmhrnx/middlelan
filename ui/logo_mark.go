package ui

import (
	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
)

// logoMark draws the MiddleLAN emblem, the "M" whose strokes meet in the
// node in the middle, at the given size.
func logoMark(size int) giode.Element {
	return giode.Box(giode.Styles{
		Width: props.Width(size), Height: props.Height(size), BorderRadius: props.BorderRadius(size * 26 / 100),
		BackgroundGradient: giode.Linear(135, "#0d1330", "#241052"),
		BoxShadow:          giode.BoxShadow{Y: 4, Blur: 14, Color: "#22d3ee33"},
	}, giode.Canvas(giode.Styles{Width: props.Width(size), Height: props.Height(size)}, func(p *giode.Painter) {
		s := p.Width() / 1024
		pt := func(x, y float32) (float32, float32) { return x * s, (y - 24) * s }
		path := p.Path()
		path.MoveTo(pt(262, 730))
		path.LineTo(pt(330, 318))
		path.LineTo(pt(512, 566))
		path.LineTo(pt(694, 318))
		path.LineTo(pt(762, 730))
		path.Stroke(64*s, "#7dd3fc")
		cx, cy := pt(512, 566)
		p.Circle(cx, cy, 128*s, "#0f1232")
		p.Arc(cx, cy, 128*s, 16*s, 0, 360, "#818cf8")
		p.Circle(cx, cy, 76*s, "#a5f3fc")
		lx, ly := pt(262, 730)
		p.Circle(lx, ly, 56*s, "#22d3ee")
		rx, ry := pt(762, 730)
		p.Circle(rx, ry, 56*s, "#c084fc")
	}))
}
