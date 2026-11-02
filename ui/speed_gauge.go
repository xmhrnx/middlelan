package ui

import (
	"math"

	"github.com/xmhrnx/giode"
)

// speedGauge is a 270° dial for a speed in Mbps, on a log scale up to
// 1 Gbps so slow and fast connections both read well.
func speedGauge(mbps float64, label string, c giode.Color) giode.Element {
	frac := 0.0
	if mbps > 0 {
		frac = math.Min(math.Log10(1+mbps)/math.Log10(1001), 1)
	}
	value := "—"
	if mbps > 0 {
		value = trimRate(mbps)
	}
	return giode.Box(giode.Styles{Width: 240, Height: 220, Position: giode.PositionRelative, Align: giode.AlignCenter, Justify: giode.JustifyCenter},
		giode.Canvas(giode.Styles{Width: 240, Height: 220}, func(p *giode.Painter) {
			cx, cy, r := float32(120), float32(120), float32(96)
			p.Arc(cx, cy, r, 14, 225, 270, colGrid)
			if frac > 0 {
				p.Arc(cx, cy, r, 14, 225, float32(270*frac), c)
			}
			for i := 0; i <= 10; i++ {
				a := (225 + 27*float64(i)) * math.Pi / 180
				x1, y1 := cx+float32(math.Sin(a))*(r-20), cy-float32(math.Cos(a))*(r-20)
				x2, y2 := cx+float32(math.Sin(a))*(r-26), cy-float32(math.Cos(a))*(r-26)
				p.Line(x1, y1, x2, y2, 2, colGrid)
			}
		}),
		giode.Stack(giode.Styles{Position: giode.PositionAbsolute, Top: 82, Left: giode.Flush, Right: giode.Flush, Align: giode.AlignCenter, Gap: 2},
			giode.Text(value, giode.Styles{Color: "theme.text", FontSize: 40, FontWeight: giode.FontWeightBold}),
			giode.Text("Mbps", styleMuted),
			giode.Text(label, giode.Styles{Color: c, FontSize: sizeSmall, FontWeight: giode.FontWeightSemiBold}),
		),
	)
}
