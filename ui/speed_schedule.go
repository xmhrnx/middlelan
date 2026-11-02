package ui

import (
	"context"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/nettools"
	"github.com/xmhrnx/middlelan/internal/settings"
	"github.com/xmhrnx/middlelan/internal/speedlog"
)

// recordSpeed saves a speed test result with the current provider.
func (sh *Shell) recordSpeed(res nettools.SpeedResult, scheduled bool) error {
	isp := ""
	if p := sh.public.Load(); p != nil {
		isp = nonEmpty(p.ISP, p.Org)
	}
	return sh.svc.Speed.Add(speedlog.Result{
		When: res.When, DownloadMbps: res.DownloadMbps, UploadMbps: res.UploadMbps,
		LatencyMs: res.LatencyMs, JitterMs: res.JitterMs, Scheduled: scheduled, ISP: isp,
	})
}

// runSpeedSchedule runs scheduled speed tests until ctx ends, checking
// every few minutes whether one is due.
func (sh *Shell) runSpeedSchedule(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
		st := sh.svc.Settings.Get()
		if !st.ScheduledSpeedTests {
			continue
		}
		last, _ := sh.svc.Speed.Last()
		if !speedlog.Due(last.When, time.Duration(st.SpeedEveryHours)*time.Hour, time.Now()) {
			continue
		}
		tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		res, err := nettools.SpeedTest(tctx, nil)
		cancel()
		if err == nil {
			err = sh.recordSpeed(res, true)
		}
		if err != nil && ctx.Err() == nil {
			sh.Toast(giode.ToastWarning, tr("Scheduled speed test failed: {error}", "error", err.Error()))
		}
		sh.Redraw()
	}
}

// updateSettings changes and saves settings, then applies them.
func (sh *Shell) updateSettings(fn func(*settings.Settings)) {
	if err := sh.svc.Settings.Update(fn); err != nil {
		sh.Toast(giode.ToastError, tr("Couldn't save settings: {error}", "error", err.Error()))
	}
	sh.ApplySettings()
}
