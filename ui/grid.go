package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mehran9675/giode"
	props "github.com/mehran9675/giode/styles/properties"
)

// grid lays children out in rows of cols equal columns. The cells of a
// row share the tallest one's height, so cards line up at the bottom.
func grid(cols, gap int, children ...giode.Element) giode.Element {
	if cols < 1 {
		cols = 1
	}
	rows := make([]giode.Element, 0, (len(children)+cols-1)/cols)
	for i := 0; i < len(children); i += cols {
		end := min(i+cols, len(children))
		rows = append(rows, equalRow(cols, gap, children[i:end]))
	}
	return giode.Stack(giode.Styles{Gap: props.Gap(gap), Width: -1}, rows...)
}

// equalRow lays the row out once off screen to find its height, then for
// real with every card in it stretched to that height.
func equalRow(cols, gap int, cells []giode.Element) giode.Element {
	return giode.Raw(func(gtx layout.Context) layout.Dimensions {
		minH := 0
		build := func() giode.Element {
			kids := make([]giode.Element, cols)
			for j := range kids {
				var child giode.Element = giode.Box(giode.Styles{})
				if j < len(cells) {
					child = cells[j]
				}
				if c, ok := child.(cardElement); ok && minH > 0 {
					child = c.withHeight(int(float32(minH)/gtx.Metric.PxPerDp + 0.5))
				}
				kids[j] = giode.Stack(giode.Styles{FlexGrow: 1, Width: -1}, child)
			}
			return giode.Row(giode.Styles{Gap: props.Gap(gap), Wrap: giode.WrapNoWrap, Width: -1}, kids...)
		}
		measure := gtx
		measure.Ops = new(op.Ops)
		minH = build().Layout(measure).Size.Y
		return build().Layout(gtx)
	})
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

// equalHeight lays a row out twice: once, off screen, to find its height,
// then for real with that height as every child's minimum, so the cards
// in a grid row line up at the bottom.
func equalHeight(row giode.Element) giode.Element {
	return giode.Raw(func(gtx layout.Context) layout.Dimensions {
		measure := gtx
		measure.Ops = new(op.Ops)
		h := row.Layout(measure).Size.Y
		gtx.Constraints.Min.Y = h
		return row.Layout(gtx)
	})
}
