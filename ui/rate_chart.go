package ui

import (
	"math"

	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// rateChart draws download and upload throughput over time: soft areas
// under crisp lines, on a quiet grid, with the scale's top value labeled.
func rateChart(points []monitor.RatePoint, height int) giode.Element {
	peak := 0.0
	for _, p := range points {
		peak = math.Max(peak, math.Max(p.Rx, p.Tx))
	}
	top := niceCeil(peak)
	return giode.Stack(giode.Styles{Gap: 6, Width: -1},
		giode.Row(giode.Styles{Gap: 16, Align: giode.AlignCenter},
			legendDot(tr("Download"), colDown),
			legendDot(tr("Upload"), colUp),
			giode.Box(giode.Styles{FlexGrow: 1}),
			giode.Text(tr("Scale {rate}", "rate", formatRate(top)), styleMuted),
		),
		giode.Canvas(giode.Styles{Width: -1, Height: props.Height(height)}, func(p *giode.Painter) {
			w, h := p.Width(), p.Height()
			for i := 0; i <= 4; i++ {
				y := h * float32(i) / 4
				p.Line(0, y, w, y, 1, colGrid)
			}
			n := len(points)
			if n < 2 || top <= 0 {
				return
			}
			xs := func(i int) float32 { return w * float32(i) / float32(n-1) }
			ys := func(v float64) float32 { return h - float32(v/top)*(h-4) - 1 }
			series := func(get func(monitor.RatePoint) float64, line, fill giode.Color) {
				area := p.Path()
				area.MoveTo(0, h)
				for i, pt := range points {
					area.LineTo(xs(i), ys(get(pt)))
				}
				area.LineTo(w, h)
				area.Close()
				area.Fill(fill)
				stroke := p.Path()
				for i, pt := range points {
					if i == 0 {
						stroke.MoveTo(xs(i), ys(get(pt)))
					} else {
						stroke.LineTo(xs(i), ys(get(pt)))
					}
				}
				stroke.Stroke(2, line)
			}
			series(func(r monitor.RatePoint) float64 { return r.Rx }, colDown, giode.Mix("theme.primary", "theme.surface", 0.18))
			series(func(r monitor.RatePoint) float64 { return r.Tx }, colUp, giode.Mix("theme.accent", "theme.surface", 0.14))
			last := points[n-1]
			p.Circle(w-3, ys(last.Rx), 4, colDown)
			p.Circle(w-3, ys(last.Tx), 4, colUp)
		}),
	)
}

// legendDot is a colored dot with a label.
func legendDot(label string, c giode.Color) giode.Element {
	return giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{Width: 10, Height: 10, BorderRadius: 3, Background: c}),
		giode.Text(label, styleMuted),
	)
}

// niceCeil rounds a rate (bytes/s) up to a round number of bits per
// second, so the chart's scale reads well. It never goes below 8 Kbps.
func niceCeil(v float64) float64 {
	bits := math.Max(v*8, 8000)
	mag := math.Pow(10, math.Floor(math.Log10(bits)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if bits <= m*mag {
			return m * mag / 8
		}
	}
	return 10 * mag / 8
}
