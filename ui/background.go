package ui

import (
	"context"
	"sync"
	"time"

	"github.com/xmhrnx/middlelan/internal/monitor"
)

// Start runs the UI's background jobs until ctx ends: scheduled speed
// tests and update checks.
func (sh *Shell) Start(ctx context.Context) {
	go sh.runSpeedSchedule(ctx)
	go sh.runUpdateChecks(ctx)
}

// widgetEvery limits how often the Android widget is redrawn.
const widgetEvery = 30 * time.Second

var widgetMu sync.Mutex
var widgetLast time.Time

// OnSample runs after each traffic sample, on the collector's goroutine:
// it feeds the home screen widget.
func (sh *Shell) OnSample(s *monitor.Snapshot) {
	widgetMu.Lock()
	due := time.Since(widgetLast) >= widgetEvery
	if due {
		widgetLast = time.Now()
	}
	widgetMu.Unlock()
	if !due {
		return
	}
	top := ""
	if len(s.Apps) > 0 && s.Apps[0].TodayRx+s.Apps[0].TodayTx > 0 {
		top = tr("Top: {app} · {amount}", "app", s.Apps[0].Name, "amount", formatBytes(s.Apps[0].TodayRx+s.Apps[0].TodayTx))
	}
	_ = pushWidget(formatRate(s.RateRx), formatRate(s.RateTx), formatBytes(s.TodayRx+s.TodayTx), top) // the widget is optional
}

// ShowApp brings the window up on an app's page (from the tray).
func (sh *Shell) ShowApp(name string) {
	sh.svc.App.Post(func() {
		sh.svc.App.ShowWindow()
		sh.openApp(name)
	})
}
