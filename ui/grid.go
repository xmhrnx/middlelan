package ui

import (
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// grid lays children out in rows of cols equal columns.
func grid(cols, gap int, children ...giode.Element) giode.Element {
	if cols < 1 {
		cols = 1
	}
	rows := make([]giode.Element, 0, (len(children)+cols-1)/cols)
	for i := 0; i < len(children); i += cols {
		cells := make([]giode.Element, 0, cols)
		for j := 0; j < cols; j++ {
			var child giode.Element = giode.Box(giode.Styles{})
			if i+j < len(children) {
				child = children[i+j]
			}
			cells = append(cells, giode.Stack(giode.Styles{FlexGrow: 1, Width: -1}, child))
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: props.Gap(gap), Wrap: giode.WrapNoWrap, Width: -1}, cells...))
	}
	return giode.Stack(giode.Styles{Gap: props.Gap(gap), Width: -1}, rows...)
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
