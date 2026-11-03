package ui

import (
	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
)

// logoMark draws the MiddleMan emblem, the man in the middle between two
// linked devices, at the given size (assets/src/make_icons.py draws the
// same at full detail).
func logoMark(size int) giode.Element {
	return giode.Box(giode.Styles{
		Width: props.Width(size), Height: props.Height(size), BorderRadius: props.BorderRadius(size * 26 / 100),
		BackgroundGradient: giode.Linear(180, "#202021", "#0c0c0d"),
		BorderWidth:        1, BorderColor: "#ffffff1f",
		BoxShadow: giode.BoxShadow{Y: 4, Blur: 14, Color: "#00000055"},
	}, giode.Canvas(giode.Styles{Width: props.Width(size), Height: props.Height(size)}, func(p *giode.Painter) {
		s := p.Width() / 1024
		const gold, dark, pearl = "#ddb46a", "#141415", "#f3e6c8"
		// Drawn a little larger than the icon, so it reads at sidebar size.
		pt := func(x, y float32) (float32, float32) { return (512 + (x-512)*1.12) * s, (512 + (y-540)*1.12) * s }
		r := func(v float32) float32 { return v * 1.12 * s }
		path := func(fill string, pts ...[2]float32) {
			pa := p.Path()
			pa.MoveTo(pt(pts[0][0], pts[0][1]))
			for _, q := range pts[1:] {
				pa.LineTo(pt(q[0], q[1]))
			}
			pa.Close()
			pa.Fill(giode.Color(fill))
		}
		hx, hy := pt(512, 392)
		p.Circle(hx, hy, r(96), gold)
		body := p.Path()
		body.MoveTo(pt(340, 742))
		body.LineTo(pt(340, 684))
		x1, y1 := pt(340, 600)
		x2, y2 := pt(404, 540)
		x3, y3 := pt(512, 532)
		body.CubeTo(x1, y1, x2, y2, x3, y3)
		x1, y1 = pt(620, 540)
		x2, y2 = pt(684, 600)
		x3, y3 = pt(684, 684)
		body.CubeTo(x1, y1, x2, y2, x3, y3)
		body.LineTo(pt(684, 742))
		body.Close()
		body.Fill(gold)
		path(dark, [2]float32{466, 536}, [2]float32{512, 650}, [2]float32{558, 536})
		path(pearl, [2]float32{512, 566}, [2]float32{530, 590}, [2]float32{520, 604}, [2]float32{534, 700},
			[2]float32{512, 726}, [2]float32{490, 700}, [2]float32{504, 604}, [2]float32{494, 590})
		for _, x := range []float32{150, 874} {
			cx, cy := pt(x, 640)
			p.Circle(cx, cy, r(50), gold)
			p.Circle(cx, cy, r(38), dark)
			p.Circle(cx, cy, r(16), pearl)
		}
	}))
}
