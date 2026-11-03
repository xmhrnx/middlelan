package ui

import (
	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
)

// logoMark draws the MiddleMan emblem, the "M" whose strokes meet in the
// node in the middle, at the given size.
func logoMark(size int) giode.Element {
	return giode.Box(giode.Styles{
		Width: props.Width(size), Height: props.Height(size), BorderRadius: props.BorderRadius(size * 26 / 100),
		BackgroundGradient: giode.Linear(180, "#24211d", "#0e0d0c"),
		BorderWidth:        1, BorderColor: "#e9c77f40",
		BoxShadow: giode.BoxShadow{Y: 4, Blur: 14, Color: "#00000055"},
	}, giode.Canvas(giode.Styles{Width: props.Width(size), Height: props.Height(size)}, func(p *giode.Painter) {
		s := p.Width() / 1024
		pt := func(x, y float32) (float32, float32) { return x * s, (y - 24) * s }
		path := p.Path()
		path.MoveTo(pt(300, 742))
		path.LineTo(pt(300, 292))
		path.LineTo(pt(512, 552))
		path.LineTo(pt(724, 292))
		path.LineTo(pt(724, 742))
		path.Stroke(64*s, "#ddb46a")
		cx, cy := pt(512, 552)
		p.Circle(cx, cy, 112*s, "#121110")
		p.Arc(cx, cy, 86*s, 14*s, 0, 360, "#ddb46a")
		p.Circle(cx, cy, 48*s, "#f3e6c8")
	}))
}
