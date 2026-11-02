package ui

import (
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/inventory"
)

// presenceStrip shows when a device was on the network over the last
// `span`: filled where it was online, from its join/leave events.
func presenceStrip(events []inventory.Event, online bool, span time.Duration, now time.Time) giode.Element {
	return giode.Canvas(giode.Styles{Width: -1, Height: 14}, func(p *giode.Painter) {
		w := p.Width()
		start := now.Add(-span)
		p.Rect(0, 3, w, 8, 4, colGrid)
		x := func(t time.Time) float32 {
			if t.Before(start) {
				t = start
			}
			return w * float32(t.Sub(start)) / float32(span)
		}
		// Walk the events, tracking the state since the strip's start.
		state, since := false, start
		for _, e := range events {
			if e.At.Before(start) {
				state = e.Online
				continue
			}
			if state {
				p.Rect(x(since), 3, max(x(e.At)-x(since), 2), 8, 4, "theme.success")
			}
			state, since = e.Online, e.At
		}
		if state || (len(events) == 0 && online) {
			p.Rect(x(since), 3, max(w-x(since), 2), 8, 4, "theme.success")
		}
	})
}
