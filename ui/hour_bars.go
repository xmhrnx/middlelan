package ui

import (
	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
)

// barSeries is one bar per period (an hour or a day), download and
// upload stacked.
type barSeries struct {
	rx, tx   []uint64
	selected int // highlighted bar, -1 for none
}

// stackedBars draws the series; the caller wraps it in a Pressable to pick
// a bar (barAt maps a click to one).
func stackedBars(s barSeries, height int) giode.Element {
	var peak uint64
	for i := range s.rx {
		peak = max(peak, s.rx[i]+s.tx[i])
	}
	return giode.Canvas(giode.Styles{Width: -1, Height: props.Height(height)}, func(p *giode.Painter) {
		w, h := p.Width(), p.Height()
		p.Line(0, h-0.5, w, h-0.5, 1, colGrid)
		n := len(s.rx)
		if n == 0 || peak == 0 {
			return
		}
		slot := w / float32(n)
		bar := max(slot*0.7, 1)
		for i := 0; i < n; i++ {
			x := slot*float32(i) + (slot-bar)/2
			if i == s.selected {
				p.Rect(slot*float32(i), 0, slot, h, 3, colSelected)
			}
			rxH := float32(float64(s.rx[i])/float64(peak)) * (h - 2)
			txH := float32(float64(s.tx[i])/float64(peak)) * (h - 2)
			if rxH > 0 {
				p.Rect(x, h-rxH, bar, rxH, min(bar/3, 3), colDown)
			}
			if txH > 0 {
				p.Rect(x, h-rxH-txH, bar, txH, min(bar/3, 3), colUp)
			}
		}
	})
}

// barAt returns the bar under x (dp) in a chart w dp wide with n bars.
func barAt(x, w float32, n int) int {
	if n == 0 || w <= 0 {
		return -1
	}
	i := int(x / (w / float32(n)))
	return min(max(i, 0), n-1)
}
