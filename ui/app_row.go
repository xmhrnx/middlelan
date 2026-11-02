package ui

import (
	"github.com/mehran9675/giode"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// appRow is one program in a list: its avatar, name, activity and rate.
// share (0..1) draws a bar of its part of the traffic; negative hides it.
func appRow(a monitor.App, share float64, s giode.PressState, compact bool) giode.Element {
	bg := giode.Color("transparent")
	if s.Hovered || s.Pressed {
		bg = colHover
	}
	sub := plural(a.Active, "connection", "connections")
	if a.Listening > 0 {
		sub += " · " + plural(a.Listening, "listening port", "listening ports")
	}
	info := []giode.Element{
		giode.Text(a.Name, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightSemiBold, TextOverflow: giode.TextOverflowEllipsis}),
		giode.Text(sub, giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
	}
	if share >= 0 {
		info = append(info, shareBar(share))
	}
	kids := []giode.Element{
		giode.Avatar(a.Name, giode.Styles{Width: 36}),
		giode.Stack(giode.Styles{Gap: 4, FlexGrow: 1, Width: -1}, info...),
	}
	if !compact && len(a.History) > 1 {
		kids = append(kids, sparkline(a.History, 90, 28, colDown))
	}
	rate := "—"
	if a.KnownBytes {
		rate = formatRate(a.RateRx + a.RateTx)
	}
	kids = append(kids, giode.Stack(giode.Styles{Align: giode.AlignEnd, Gap: 3, MinWidth: 84},
		giode.Text(rate, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontWeight: giode.FontWeightSemiBold}),
		giode.Text(tr("{amount} today", "amount", formatBytes(a.TodayRx+a.TodayTx)), giode.Styles{Color: "theme.muted", FontSize: sizeTiny}),
	))
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 12, Padding: giode.SymmetricInset(10, 9), BorderRadius: 12, Background: bg, Width: -1, Wrap: giode.WrapNoWrap}, kids...)
}

// shareBar is a thin bar filled to share (0..1).
func shareBar(share float64) giode.Element {
	return giode.Canvas(giode.Styles{Width: -1, Height: 4}, func(p *giode.Painter) {
		w := p.Width()
		p.Rect(0, 0, w, 4, 2, colGrid)
		if share > 0 {
			p.Rect(0, 0, max(4, w*float32(min(share, 1))), 4, 2, colDown)
		}
	})
}
