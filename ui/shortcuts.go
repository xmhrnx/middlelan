package ui

import (
	"strconv"

	"github.com/xmhrnx/giode"
)

// navShortcut is the key that opens the i-th page of the navigation:
// Mod+1 for Overview, Mod+2 for Apps, and so on.
func navShortcut(i int) string { return "Mod+" + strconv.Itoa(i+1) }

// newShortcuts builds the app-wide keyboard shortcuts.
func newShortcuts(sh *Shell) []giode.KeyShortcut {
	list := []giode.KeyShortcut{
		giode.Shortcut("Mod+,", func() { sh.navigate(screenSettings) }),
		giode.Shortcut("Mod+P", sh.togglePause),
		giode.Shortcut("F5", func() {
			sh.svc.Collector.Refresh()
			if sh.current == screenDevices {
				sh.svc.Scanner.Start(sh.ctx)
			}
		}),
		giode.Shortcut("Mod+F", sh.focusSearch),
	}
	for i, n := range navItems {
		if n.screen == screenSettings {
			continue
		}
		s := n.screen
		list = append(list, giode.Shortcut(navShortcut(i), func() { sh.navigate(s) }))
	}
	return list
}

// focusSearch puts the cursor in the current page's search field,
// opening Apps when the page has none.
func (sh *Shell) focusSearch() {
	switch sh.current {
	case screenConnections:
		sh.connections.search.Focus()
	default:
		sh.navigate(screenApps)
		sh.apps.search.Focus()
	}
}
