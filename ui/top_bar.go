package ui

import "github.com/mehran9675/giode"

// topBar is the phone header: the brand, live rates and settings.
type topBar struct {
	shell    *Shell
	settings *giode.PressableElement
}

func newTopBar(sh *Shell) *topBar {
	t := &topBar{shell: sh}
	t.settings = giode.Pressable(func(s giode.PressState) giode.Element {
		bg := giode.Color("transparent")
		if s.Hovered || s.Pressed || sh.current == screenSettings {
			bg = colHover
		}
		return giode.Box(giode.Styles{Padding: giode.UniformInset(8), BorderRadius: 10, Background: bg},
			giode.Icon("settings", giode.Styles{Width: 20, Color: "theme.text"}))
	}).Label("Settings").OnClick(func(giode.ClickEvent) { sh.navigate(screenSettings) })
	return t
}

func (t *topBar) view() giode.Element {
	snap := t.shell.svc.Collector.Snapshot()
	return giode.Row(giode.Styles{
		Width: -1, Align: giode.AlignCenter, Gap: 10, Padding: giode.SymmetricInset(14, 10), Background: colSidebar, Wrap: giode.WrapNoWrap,
	},
		logoMark(28),
		giode.Text("MiddleLAN", giode.Styles{Color: "theme.text", FontSize: 16, FontWeight: giode.FontWeightBold, FlexGrow: 1}),
		ratePair(formatRate(snap.RateRx), formatRate(snap.RateTx), 11),
		t.settings,
	)
}
