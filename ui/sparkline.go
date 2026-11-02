package ui

import (
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// sparkline is a tiny trend line of recent values.
func sparkline(values []float64, w, h int, c giode.Color) giode.Element {
	return giode.Canvas(giode.Styles{Width: props.Width(w), Height: props.Height(h)}, func(p *giode.Painter) {
		peak := 0.0
		for _, v := range values {
			if v > peak {
				peak = v
			}
		}
		fw, fh := p.Width(), p.Height()
		if len(values) < 2 || peak == 0 {
			p.Line(0, fh-1, fw, fh-1, 1.5, colGrid)
			return
		}
		path := p.Path()
		for i, v := range values {
			x := fw * float32(i) / float32(len(values)-1)
			y := fh - 1 - float32(v/peak)*(fh-3)
			if i == 0 {
				path.MoveTo(x, y)
			} else {
				path.LineTo(x, y)
			}
		}
		path.Stroke(1.6, c)
	})
}
