package ui

import "github.com/xmhrnx/giode"

// toolFrame is the common layout of a tool page: the title with a back
// button, the form, then the results.
func (t *toolsScreen) toolFrame(info toolInfo, form giode.Element, results ...giode.Element) giode.Element {
	head := giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
		iconBadge(info.icon, info.color, 46),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Text(tr(info.name), styleTitle),
			giode.Text(tr(info.desc), giode.Styles{Color: "theme.muted", FontSize: sizeSmall}),
		),
	)
	kids := []giode.Element{giode.Row(giode.Styles{}, t.back), head}
	if form != nil {
		kids = append(kids, cardStyled(giode.Styles{}, form))
	}
	kids = append(kids, results...)
	return pageBody(kids...)
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
		b.Label(tr("Stop"))
	} else {
		b.Label(tr(label))
	}
	return b
}
