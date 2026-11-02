package ui

import (
	"time"

	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// usageBars is a bar per day, download and upload side by side, with the
// weekday under each.
func usageBars(days []monitor.DayUsage, height int) giode.Element {
	var peak uint64
	for _, d := range days {
		peak = max(peak, d.Rx, d.Tx)
	}
	labels := make([]giode.Element, len(days))
	for i, d := range days {
		name := d.Day
		if t, err := time.Parse("2006-01-02", d.Day); err == nil {
			name = t.Format("Mon")
		}
		labels[i] = giode.Text(name, giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextAlign: giode.TextAlignCenter, FlexGrow: 1, Width: -1})
	}
	return giode.Stack(giode.Styles{Gap: 6, Width: -1},
		giode.Canvas(giode.Styles{Width: -1, Height: props.Height(height)}, func(p *giode.Painter) {
			w, h := p.Width(), p.Height()
			p.Line(0, h-0.5, w, h-0.5, 1, colGrid)
			if peak == 0 || len(days) == 0 {
				return
			}
			slot := w / float32(len(days))
			bar := min(slot*0.26, 18)
			for i, d := range days {
				cx := slot*float32(i) + slot/2
				for j, v := range []uint64{d.Rx, d.Tx} {
					if v == 0 {
						continue
					}
					bh := float32(float64(v)/float64(peak)) * (h - 2)
					if bh < 3 {
						bh = 3
					}
					x := cx - bar - 1.5
					c := colDown
					if j == 1 {
						x, c = cx+1.5, colUp
					}
					p.Rect(x, h-bh, bar, bh, min(bar/2, 5), c)
				}
			}
		}),
		giode.Row(giode.Styles{Width: -1, Wrap: giode.WrapNoWrap}, labels...),
	)
}
