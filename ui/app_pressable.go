package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// appPressable is a clickable appRow that opens the app's details. Rows
// are kept by (scope, name) so their press state survives re-sorting; each
// frame they draw the latest data for that app.
func (sh *Shell) appPressable(scope, name string, share float64) giode.Element {
	type rowState struct {
		p     *giode.PressableElement
		share float64
	}
	st := giode.Keyed("app-row-"+scope, name, func() *rowState {
		rs := &rowState{}
		rs.p = giode.Pressable(func(s giode.PressState) giode.Element {
			a, ok := sh.findApp(name)
			if !ok {
				a = monitor.App{Name: name}
			}
			return appRow(a, rs.share, s, giode.Compact())
		}).Label(name).OnClick(func(e giode.ClickEvent) {
			if e.Button != giode.MouseSecondary { // a right-click opens the menu
				sh.openApp(name)
			}
		})
		return rs
	})
	st.share = share
	st.p.Context(sh.appMenu(name))
	return st.p
}

// findApp looks an app up in the latest snapshot.
func (sh *Shell) findApp(name string) (monitor.App, bool) {
	for _, a := range sh.svc.Collector.Snapshot().Apps {
		if a.Name == name {
			return a, true
		}
	}
	return monitor.App{}, false
}
