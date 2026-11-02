package ui

import (
	"gioui.org/layout"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/titlebar"
)

// newWindowBar builds the custom title bar of the frameless desktop window:
// the emblem and name, live rates, and the window buttons (traffic lights
// on macOS). Closing hides to the tray when that's enabled, so monitoring
// keeps running. Phones have no window frame and get nil.
func newWindowBar(sh *Shell) *titlebar.TitleBar {
	if isMobile {
		return nil
	}
	bar := giode.TitleBar(sh.svc.App, "MiddleLAN", giode.Styles{
		Background: colSidebar, Color: "theme.muted", Height: 38, FontSize: sizeSmall,
		FontWeight: giode.FontWeightSemiBold, BorderWidth: 1, BorderColor: "theme.border",
	}).
		Variant(titleBarVariant).
		Icon(logoMark(18)).
		Trailing(giode.Raw(func(gtx layout.Context) layout.Dimensions {
			snap := sh.svc.Collector.Snapshot()
			return giode.Box(giode.Styles{Padding: giode.SymmetricInset(12, 0)},
				ratePair(formatRate(snap.RateRx), formatRate(snap.RateTx), 11)).Layout(gtx)
		})).
		OnClose(func() bool {
			if sh.svc.App.CloseToTray() {
				sh.svc.App.HideWindow()
				sh.svc.Collector.SetVisible(false)
				return false
			}
			return true
		})
	return bar
}
