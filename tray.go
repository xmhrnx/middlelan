package main

import (
	"sync"
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/middlelan/assets"
	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/ui"
)

// trayPanel is MiddleMan in the system tray (the menu bar on macOS): a
// small live panel in the menu - speeds, today's total, the busiest apps
// - so a glance doesn't need the window.
type trayPanel struct {
	app   *giode.App
	c     *monitor.Collector
	shell *ui.Shell

	mu       sync.Mutex
	lastMenu string
	lastSet  time.Time
}

// trayEvery limits menu rebuilds; some desktops redraw the whole menu.
const trayEvery = 4 * time.Second

func setupTray(app *giode.App, c *monitor.Collector, sh *ui.Shell) *trayPanel {
	if !giode.TraySupported() {
		return nil
	}
	t := &trayPanel{app: app, c: c, shell: sh}
	app.Tray().SetIcon(assets.Tray).SetTemplateIcon(assets.TrayTemplate).SetTooltip("MiddleMan")
	t.refresh(true)
	return t
}

// refresh rebuilds the menu from the latest snapshot when it changed.
func (t *trayPanel) refresh(force bool) {
	if t == nil {
		return
	}
	s := t.c.Snapshot()
	rates := "↓ " + ui.FormatRate(s.RateRx) + "    ↑ " + ui.FormatRate(s.RateTx)
	today := giode.Intl("Today {amount}", "amount", ui.FormatBytes(s.TodayRx+s.TodayTx))
	items := []giode.TrayItem{
		{Label: rates, Disabled: true},
		{Label: today, Disabled: true},
		giode.TraySeparator(),
	}
	key := rates + today
	shown := 0
	for _, a := range s.Apps {
		if shown == 3 || a.RateRx+a.RateTx+float64(a.TodayRx+a.TodayTx) == 0 {
			break
		}
		name := a.Name
		label := name + "   " + ui.FormatRate(a.RateRx+a.RateTx)
		items = append(items, giode.TrayAction(label, func() { t.shell.ShowApp(name) }))
		key += label
		shown++
	}
	if shown > 0 {
		items = append(items, giode.TraySeparator())
	}
	pause := giode.Intl("Pause monitoring")
	if t.c.Paused() {
		pause = giode.Intl("Resume monitoring")
	}
	items = append(items,
		giode.TrayAction(giode.Intl("Open MiddleMan"), func() {
			t.app.ShowWindow()
			t.c.SetVisible(true)
		}),
		giode.TrayAction(pause, func() {
			t.c.SetPaused(!t.c.Paused())
			t.refresh(true)
		}),
		giode.TraySeparator(),
		giode.TrayAction(giode.Intl("Quit"), t.app.Quit),
	)
	key += pause
	t.mu.Lock()
	changed := key != t.lastMenu && (force || time.Since(t.lastSet) >= trayEvery)
	if changed {
		t.lastMenu, t.lastSet = key, time.Now()
	}
	t.mu.Unlock()
	if changed {
		t.app.Tray().SetMenu(items...).SetTooltip("MiddleMan  " + rates)
		// macOS shows a short live title next to the menu bar icon.
		t.app.Tray().SetTitle("↓" + ui.FormatRate(s.RateRx))
	}
}
