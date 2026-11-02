package ui

import (
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
	"github.com/xmhrnx/middlelan/internal/inventory"
)

// deviceRow is one device on the network.
func deviceRow(d inventory.Device, s giode.PressState, now time.Time) giode.Element {
	bg := giode.Color("transparent")
	if s.Hovered || s.Pressed {
		bg = colHover
	}
	accent := giode.Color("theme.primary")
	switch {
	case d.IsGateway:
		accent = "theme.warning"
	case d.IsSelf:
		accent = "theme.success"
	case !d.Online:
		accent = "theme.muted"
	}
	sub := []string{d.IP}
	if d.Vendor != "" {
		sub = append(sub, d.Vendor)
	}
	if d.MAC != "" {
		sub = append(sub, strings.ToUpper(d.MAC))
	}
	tags := []giode.Element{}
	switch {
	case d.IsSelf:
		tags = append(tags, pill(tr("This device"), "theme.success"))
	case d.IsGateway:
		tags = append(tags, pill(tr("Router"), "theme.warning"))
	}
	if !d.Trusted && !d.IsSelf && now.Sub(d.FirstSeen) < 24*time.Hour && !d.FirstSeen.IsZero() {
		tags = append(tags, pill(tr("New"), "theme.info"))
	}
	if !d.Online {
		tags = append(tags, giode.Text(tr("Seen {ago}", "ago", formatAgo(d.LastSeen, now)), styleMuted))
	} else if d.LatencyMs > 0 {
		tags = append(tags, giode.Text(trimRate(d.LatencyMs)+" ms", styleMuted))
	}
	opacity := float32(1)
	if !d.Online {
		opacity = 0.6
	}
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 12, Padding: giode.SymmetricInset(10, 9), BorderRadius: 12, Background: bg, Width: -1, Wrap: giode.WrapNoWrap, Opacity: props.Opacity(opacity)},
		iconBadge(deviceIcon(d.Kind), accent, 38),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Text(d.Name(), giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightSemiBold, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(strings.Join(sub, " · "), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
		),
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, tags...),
		giode.Icon(chevronForward(), giode.Styles{Width: 16, Color: "theme.muted"}),
	)
}
