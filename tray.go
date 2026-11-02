package main

import (
	"github.com/mehran9675/giode"

	"github.com/xmhrnx/middlelan/assets"
	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/ui"
)

// setupTray puts MiddleLAN in the system tray (the menu bar on macOS), so
// it can keep monitoring with the window closed.
func setupTray(app *giode.App, c *monitor.Collector, sh *ui.Shell) {
	if !giode.TraySupported() {
		return
	}
	show := func() {
		app.ShowWindow()
		c.SetVisible(true)
	}
	var menu func() []giode.TrayItem
	menu = func() []giode.TrayItem {
		label := "Pause monitoring"
		if c.Paused() {
			label = "Resume monitoring"
		}
		return []giode.TrayItem{
			giode.TrayAction("Open MiddleLAN", show),
			giode.TrayAction(label, func() {
				c.SetPaused(!c.Paused())
				app.Tray().SetMenu(menu()...)
			}),
			giode.TraySeparator(),
			giode.TrayAction("Quit", app.Quit),
		}
	}
	app.Tray().SetIcon(assets.Tray).SetTemplateIcon(assets.TrayTemplate).SetTooltip("MiddleLAN").SetMenu(menu()...)
}

// updateTray shows the live rates in the tray tooltip.
func updateTray(app *giode.App, c *monitor.Collector) {
	if app == nil || !giode.TraySupported() {
		return
	}
	s := c.Snapshot()
	app.Tray().SetTooltip("MiddleLAN  ↓ " + ui.FormatRate(s.RateRx) + "  ↑ " + ui.FormatRate(s.RateTx))
}
