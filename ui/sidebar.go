package ui

import (
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// sidebar is the desktop navigation: the brand, the pages and the
// monitoring status.
type sidebar struct {
	shell *Shell
	items []*giode.PressableElement
	pause *giode.PressableElement
}

func newSidebar(sh *Shell) *sidebar {
	sb := &sidebar{shell: sh}
	for i, n := range navItems {
		hint := giode.FormatShortcut(navShortcut(i))
		if n.screen == screenSettings {
			hint = giode.FormatShortcut("Mod+,")
		}
		sb.items = append(sb.items, giode.Pressable(func(s giode.PressState) giode.Element {
			return sb.itemView(n, hint, s)
		}).Label(n.label).OnClick(func(giode.ClickEvent) { sh.navigate(n.screen) }))
	}
	sb.pause = giode.Pressable(func(s giode.PressState) giode.Element {
		return sb.statusView(s)
	}).Label("Pause or resume monitoring").OnClick(func(giode.ClickEvent) { sh.togglePause() })
	return sb
}

// itemView draws a page link; hovering shows its keyboard shortcut.
func (sb *sidebar) itemView(n navItem, hint string, s giode.PressState) giode.Element {
	active := sb.shell.current == n.screen || (n.screen == screenApps && sb.shell.current == screenAppDetail)
	bg, fg, ic := giode.Color("transparent"), giode.Color("theme.muted"), giode.Color("theme.muted")
	switch {
	case active:
		bg, fg, ic = colSelected, "theme.text", "theme.primary"
	case s.Hovered:
		bg, fg = colHover, "theme.text"
	}
	return giode.Row(giode.Styles{
		Align: giode.AlignCenter, Gap: 12, Padding: giode.SymmetricInset(12, 10), Background: bg, BorderRadius: 10, Width: -1, Wrap: giode.WrapNoWrap,
	},
		giode.Box(giode.Styles{Width: 3, Height: 18, BorderRadius: 2, Background: map[bool]giode.Color{true: "theme.primary", false: "transparent"}[active]}),
		giode.Icon(n.icon, giode.Styles{Width: 18, Color: ic}),
		giode.Text(n.label, giode.Styles{Color: fg, FontSize: sizeBody, FontWeight: giode.FontWeightMedium, FlexGrow: 1}),
		giode.Text(hint, giode.Styles{Color: "theme.muted", FontSize: sizeTiny, Visibility: map[bool]props.Visibility{true: "", false: giode.VisibilityHidden}[s.Hovered]}),
	)
}

func (sb *sidebar) statusView(s giode.PressState) giode.Element {
	snap := sb.shell.svc.Collector.Snapshot()
	paused := sb.shell.svc.Collector.Paused()
	dot, label, sub := giode.Color("theme.success"), "Monitoring", "Every "+snap.Interval.String()
	if paused {
		dot, label, sub = "theme.warning", "Paused", "Click to resume"
	}
	bg := colSurface2
	if s.Hovered {
		bg = colSurface3
	}
	icon := "pause"
	if paused {
		icon = "play"
	}
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Padding: giode.UniformInset(12), Background: bg, BorderRadius: 12, Width: -1},
		statusDot(dot, !paused),
		giode.Stack(giode.Styles{FlexGrow: 1, Gap: 2},
			giode.Text(label, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontWeight: giode.FontWeightSemiBold}),
			giode.Text(sub, giode.Styles{Color: "theme.muted", FontSize: sizeTiny}),
		),
		giode.Icon(icon, giode.Styles{Width: 16, Color: "theme.muted"}),
	)
}

// Layout draws the sidebar.
func (sb *sidebar) view() giode.Element {
	snap := sb.shell.svc.Collector.Snapshot()
	kids := []giode.Element{
		giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Padding: giode.Inset{Left: 8, Bottom: 18, Top: 4}},
			logoMark(34),
			giode.Stack(giode.Styles{Gap: 1},
				giode.Text("MiddleLAN", giode.Styles{Color: "theme.text", FontSize: 17, FontWeight: giode.FontWeightBold}),
				giode.Text("Network monitor", giode.Styles{Color: "theme.muted", FontSize: sizeTiny}),
			),
		),
	}
	for _, it := range sb.items {
		kids = append(kids, it)
	}
	kids = append(kids,
		giode.Box(giode.Styles{FlexGrow: 1}),
		giode.Stack(giode.Styles{Gap: 8, Padding: giode.Inset{Bottom: 10}},
			throughputMini("Download", snap.RateRx, colDown),
			throughputMini("Upload", snap.RateTx, colUp),
		),
		sb.pause,
	)
	return giode.Stack(giode.Styles{
		Width: 236, Height: -1, Padding: giode.UniformInset(14), Gap: 4, Background: colSidebar,
		BorderColor: "theme.border",
	}, kids...)
}

// throughputMini is a one-line live rate for the sidebar.
func throughputMini(label string, rate float64, c giode.Color) giode.Element {
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 8, Padding: giode.SymmetricInset(6, 0)},
		giode.Box(giode.Styles{Width: 8, Height: 8, BorderRadius: 4, Background: c}),
		giode.Text(label, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FlexGrow: 1}),
		giode.Text(formatRate(rate), giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontWeight: giode.FontWeightSemiBold}),
	)
}
