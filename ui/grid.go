package ui

import (
	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
)

// grid lays children out in rows of cols equal columns. Rows stretch
// their cells to the tallest one, so cards side by side share a bottom
// edge.
func grid(cols, gap int, children ...giode.Element) giode.Element {
	if cols < 1 {
		cols = 1
	}
	rows := make([]giode.Element, 0, (len(children)+cols-1)/cols)
	for i := 0; i < len(children); i += cols {
		cells := make([]giode.Element, 0, cols)
		for j := 0; j < cols; j++ {
			cells = append(cells, gridCell(children, i+j))
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: props.Gap(gap), Wrap: giode.WrapNoWrap, Width: -1, Align: giode.AlignStretch}, cells...))
	}
	return giode.Stack(giode.Styles{Gap: props.Gap(gap), Width: -1}, rows...)
}

// gridCell is the i-th child sized as an equal column: a card directly,
// so the row's stretch reaches its background; anything else in a box.
func gridCell(children []giode.Element, i int) giode.Element {
	equal := giode.Styles{FlexGrow: 1, Width: -1}
	if i >= len(children) {
		return giode.Box(equal)
	}
	if c, ok := children[i].(cardElement); ok {
		return giode.Box(c.st.With(equal), c.kids...)
	}
	return giode.Stack(equal, children[i])
}

// responsiveGrid picks the column count from the available width.
func responsiveGrid(minCell, maxCols, gap int, children ...giode.Element) giode.Element {
	bps := make([]giode.Breakpoint, 0, maxCols)
	for c := 1; c < maxCols; c++ {
		cols := c
		bps = append(bps, giode.Breakpoint{MaxWidth: (c+1)*minCell + c*gap, View: func() giode.Element { return grid(cols, gap, children...) }})
	}
	bps = append(bps, giode.Breakpoint{View: func() giode.Element { return grid(maxCols, gap, children...) }})
	return giode.Responsive(bps...)
}
