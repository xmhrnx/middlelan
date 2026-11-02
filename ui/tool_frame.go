package ui

import "github.com/mehran9675/giode"

// toolFrame is the common layout of a tool page: the title with a back
// button, the form, then the results.
func (t *toolsScreen) toolFrame(info toolInfo, form giode.Element, results ...giode.Element) giode.Element {
	head := giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
		iconBadge(info.icon, info.color, 46),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter}, giode.Text(info.name, styleTitle), proBadge(info.pro)),
			giode.Text(info.desc, giode.Styles{Color: "theme.muted", FontSize: sizeSmall}),
		),
	)
	kids := []giode.Element{giode.Row(giode.Styles{}, t.back), head}
	if form != nil {
		kids = append(kids, cardStyled(giode.Styles{}, form))
	}
	kids = append(kids, results...)
	return pageBody(kids...)
}

// proBadge marks the advanced tools (which other apps sell separately;
// MiddleLAN includes them).
func proBadge(on bool) giode.Element {
	if !on {
		return giode.Box(giode.Styles{})
	}
	return giode.Box(giode.Styles{Padding: giode.SymmetricInset(7, 2), BorderRadius: 6, BackgroundGradient: gradientBrand()},
		giode.Text("PRO", giode.Styles{Color: "#ffffff", FontSize: 10, FontWeight: giode.FontWeightBold}))
}

// formRow lays out a field and its button side by side.
func formRow(kids ...giode.Element) giode.Element {
	out := []giode.Element{giode.Box(giode.Styles{FlexGrow: 1, MinWidth: 200, Width: -1}, kids[0])}
	out = append(out, kids[1:]...)
	return giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap}, out...)
}

// runButton toggles between run and stop.
func runButton(label string, onRun func(), r *toolRunner) *giode.ButtonElement {
	var b *giode.ButtonElement
	b = giode.Button(label, stylePrimary).OnClick(func() {
		if r.running {
			r.stop()
			return
		}
		onRun()
	})
	return b
}

// syncRunLabel updates a run button's label to the runner's state.
func syncRunLabel(b *giode.ButtonElement, label string, r *toolRunner) *giode.ButtonElement {
	if r.running {
		b.Label("Stop")
	} else {
		b.Label(label)
	}
	return b
}
