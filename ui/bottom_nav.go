package ui

import "github.com/xmhrnx/giode"

// bottomNav is the phone navigation bar.
type bottomNav struct {
	shell *Shell
	items []*giode.PressableElement
}

func newBottomNav(sh *Shell) *bottomNav {
	b := &bottomNav{shell: sh}
	for _, n := range navItems {
		if !n.compact {
			continue
		}
		n := n
		item := giode.Pressable(func(s giode.PressState) giode.Element {
			active := sh.current == n.screen || (n.screen == screenApps && (sh.current == screenAppDetail || sh.current == screenConnections))
			fg, bg := giode.Color("theme.muted"), giode.Color("transparent")
			if active {
				fg, bg = "theme.primary", colSelected
			}
			return giode.Stack(giode.Styles{Align: giode.AlignCenter, Gap: 3, Padding: giode.SymmetricInset(4, 8), Width: -1},
				giode.Box(giode.Styles{Padding: giode.SymmetricInset(14, 4), BorderRadius: 99, Background: bg},
					giode.Icon(n.icon, giode.Styles{Width: 20, Color: fg})),
				giode.Text(tr(n.label), giode.Styles{Color: fg, FontSize: sizeTiny, FontWeight: giode.FontWeightMedium}),
			)
		}, giode.Styles{FlexGrow: 1}).OnClick(func(giode.ClickEvent) { sh.navigate(n.screen) })
		localized(func() { item.Label(tr(n.label)) })
		b.items = append(b.items, item)
	}
	return b
}

func (b *bottomNav) view() giode.Element {
	kids := make([]giode.Element, len(b.items))
	for i, it := range b.items {
		kids[i] = it
	}
	return giode.Stack(giode.Styles{Width: -1},
		giode.Box(giode.Styles{Width: -1, Height: 1, Background: "theme.border"}),
		giode.Row(giode.Styles{Width: -1, Wrap: giode.WrapNoWrap, Background: colSidebar, Padding: giode.SymmetricInset(6, 4)}, kids...),
	)
}
