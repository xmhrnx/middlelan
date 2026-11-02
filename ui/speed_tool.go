package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/dropdown"
	"github.com/xmhrnx/giode/components/input"
	"github.com/xmhrnx/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/nettools"
	"github.com/xmhrnx/middlelan/internal/settings"
	"github.com/xmhrnx/middlelan/internal/speedlog"
)

// speedTool measures the internet connection, keeps every result, and
// grades the provider against the plan the user pays for.
type speedTool struct {
	shell    *Shell
	run      *giode.ButtonElement
	runner   toolRunner
	update   nettools.SpeedUpdate
	result   *nettools.SpeedResult
	err      string
	schedule *toggle.Switch
	every    *dropdown.Select
	planDown *input.Input
	planUp   *input.Input
}

var speedIntervals = []int{1, 3, 6, 12, 24}

func newSpeedTool(sh *Shell) *speedTool {
	s := &speedTool{shell: sh, runner: toolRunner{shell: sh}}
	s.run = runButton("Start test", s.start, &s.runner)
	cur := sh.svc.Settings.Get()
	s.schedule = switchT("Test automatically", giode.Styles{Color: "theme.text", Width: -1},
		func(on bool) { sh.updateSettings(func(st *settings.Settings) { st.ScheduledSpeedTests = on }) })
	s.schedule.SetChecked(cur.ScheduledSpeedTests)
	s.every = selectT(styleField.With(giode.Styles{Width: 170}), "Every hour", "Every 3 hours", "Every 6 hours", "Every 12 hours", "Once a day").
		OnChange(func(i int) { sh.updateSettings(func(st *settings.Settings) { st.SpeedEveryHours = speedIntervals[i] }) })
	for i, h := range speedIntervals {
		if h == cur.SpeedEveryHours {
			s.every.SetSelected(i)
		}
	}
	plan := func(placeholder string, v float64, set func(*settings.Settings, float64)) *input.Input {
		in := textInputT(placeholder, styleField.With(giode.Styles{Width: 150})).Keyboard(giode.KeyboardNumeric)
		if v > 0 {
			in.SetText(strconv.FormatFloat(v, 'f', -1, 64))
		}
		in.OnChange(func(text string) {
			f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil || f < 0 {
				f = 0
			}
			sh.updateSettings(func(st *settings.Settings) { set(st, f) })
		})
		return in
	}
	s.planDown = plan(msg("Download Mbps"), cur.PlanDownMbps, func(st *settings.Settings, f float64) { st.PlanDownMbps = f })
	s.planUp = plan(msg("Upload Mbps"), cur.PlanUpMbps, func(st *settings.Settings, f float64) { st.PlanUpMbps = f })
	return s
}

func (s *speedTool) setTarget(string) {}

func (s *speedTool) start() {
	s.update, s.result, s.err = nettools.SpeedUpdate{}, nil, ""
	s.runner.start(func(ctx context.Context) {
		res, err := nettools.SpeedTest(ctx, func(u nettools.SpeedUpdate) {
			s.runner.post(func() { s.update = u })
		})
		if err == nil {
			if serr := s.shell.recordSpeed(res, false); serr != nil {
				err = serr
			}
		}
		s.runner.post(func() {
			if err != nil {
				if ctx.Err() == nil {
					s.err = err.Error()
				}
				return
			}
			s.result = &res
		})
	})
}

func (s *speedTool) view(t *toolsScreen, info toolInfo) giode.Element {
	mbps, label, c := 0.0, tr("Ready"), colDown
	switch {
	case s.runner.running:
		mbps, label = s.update.Mbps, tr(speedPhases[s.update.Phase.String()])
		if s.update.Phase == nettools.PhaseUpload {
			c = colUp
		}
	case s.result != nil:
		mbps, label = s.result.DownloadMbps, tr("Download")
	}
	hero := cardStyled(giode.Styles{Align: giode.AlignCenter, Gap: 16, Padding: giode.UniformInset(24)},
		speedGauge(mbps, label, c),
		giode.Box(giode.Styles{Width: 280}, giode.Progress(float32(s.update.Progress), giode.Styles{Color: c, Background: colGrid, Height: 4})),
		syncRunLabel(s.run, "Start test", &s.runner),
		giode.Text(tr("Uses Cloudflare's speed test servers. A test transfers about 100 MB."), styleMuted),
	)
	results := []giode.Element{hero}
	if s.err != "" {
		results = append(results, noticeBanner("shield-alert", tr("The test failed: {error}", "error", s.err), "theme.danger", nil))
	}
	if r := s.result; r != nil {
		results = append(results, responsiveGrid(150, 4, 12,
			statTile("arrow-down", tr("Download"), fmt.Sprintf("%.1f Mbps", r.DownloadMbps), "", colDown),
			statTile("arrow-up", tr("Upload"), fmt.Sprintf("%.1f Mbps", r.UploadMbps), "", colUp),
			statTile("activity", tr("Latency"), fmt.Sprintf("%.0f ms", r.LatencyMs), tr("Idle round trip"), "theme.success"),
			statTile("signal", tr("Jitter"), fmt.Sprintf("%.1f ms", r.JitterMs), tr("Latency variation"), "theme.warning"),
		))
	}
	results = append(results, s.reportCard(), s.scheduleCard(), s.historyCard())
	return t.toolFrame(info, nil, results...)
}

// speedPhases name the speed test's phases (nettools' SpeedPhase.String).
var speedPhases = map[string]string{
	"latency": msg("Latency"), "download": msg("Download"), "upload": msg("Upload"), "done": msg("Done"), "unknown": msg("Ready"),
}

// reportSummaries are speedlog's report summaries, translated where
// they're shown.
var _ = []string{
	msg("Run a few speed tests to see how your connection holds up."),
	msg("Set your plan's speed to see whether your provider delivers it."),
	msg("Your provider delivers what you pay for."),
	msg("Mostly close to your plan, with some slow periods."),
	msg("Well below your plan. Worth raising with your provider, with this report as evidence."),
}

// reportCard grades the last 30 days of tests against the plan.
func (s *speedTool) reportCard() giode.Element {
	st := s.shell.svc.Settings.Get()
	plan := speedlog.Plan{DownloadMbps: st.PlanDownMbps, UploadMbps: st.PlanUpMbps}
	rep := speedlog.BuildReport(s.shell.svc.Speed.Results(time.Now().AddDate(0, 0, -30)), plan)
	tone := map[string]giode.Color{"A": "theme.success", "B": "theme.success", "C": "theme.warning", "D": "theme.danger", "F": "theme.danger"}[rep.Grade]
	if tone == "" {
		tone = "theme.muted"
	}
	badge := giode.Box(giode.Styles{Width: 64, Height: 64, BorderRadius: 18, Background: giode.Mix(tone, "theme.surface", 0.16), Align: giode.AlignCenter, Justify: giode.JustifyCenter},
		giode.Text(rep.Grade, giode.Styles{Color: tone, FontSize: 30, FontWeight: giode.FontWeightBold}))
	facts := []string{trn("{count} tests in the last 30 days", rep.Tests)}
	if rep.Tests > 0 {
		facts = append(facts, tr("Median {down}↓ / {up}↑ Mbps, {ping} ms", "down", fmt.Sprintf("%.0f", rep.MedianDown), "up", fmt.Sprintf("%.0f", rep.MedianUp), "ping", fmt.Sprintf("%.0f", rep.MedianPing)),
			tr("Slowest tenth: {down}↓ / {up}↑ Mbps", "down", fmt.Sprintf("%.0f", rep.P10Down), "up", fmt.Sprintf("%.0f", rep.P10Up)))
		if rep.DownPct > 0 {
			facts = append(facts, tr("{down}% of your plan's download, {up}% of its upload", "down", fmt.Sprintf("%.0f", rep.DownPct), "up", fmt.Sprintf("%.0f", rep.UpPct)))
		}
		if len(rep.SlowHours) > 0 {
			hours := make([]string, len(rep.SlowHours))
			for i, h := range rep.SlowHours {
				hours[i] = fmt.Sprintf("%02d:00", h)
			}
			facts = append(facts, tr("Slow around {hours}", "hours", strings.Join(hours, ", ")))
		}
	}
	lines := []giode.Element{giode.Text(tr(rep.Summary), giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium})}
	for _, f := range facts {
		lines = append(lines, giode.Text(f, styleMuted))
	}
	return card(tr("Provider report card"), nil,
		giode.Row(giode.Styles{Gap: 16, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
			badge, giode.Stack(giode.Styles{Gap: 4, FlexGrow: 1, Width: -1}, lines...)),
		giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter},
			giode.Text(tr("Your plan"), giode.Styles{Color: "theme.text", FontSize: sizeSmall}), s.planDown, s.planUp))
}

func (s *speedTool) scheduleCard() giode.Element {
	note := tr("Off. Each test transfers about 100 MB, so leave this off on metered connections.")
	if s.shell.svc.Settings.Get().ScheduledSpeedTests {
		note = tr("Runs in the background while MiddleLAN is open.")
		if last, ok := s.shell.svc.Speed.Last(); ok {
			note += " " + tr("Last test {ago}.", "ago", formatAgo(last.When, time.Now()))
		}
	}
	return card(tr("Scheduled tests"), nil, s.schedule,
		giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Text(tr("How often"), giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}), s.every),
		giode.Text(note, styleMuted))
}

func (s *speedTool) historyCard() giode.Element {
	results := s.shell.svc.Speed.Results(time.Now().AddDate(0, 0, -30))
	if len(results) == 0 {
		return nil
	}
	down := make([]float64, len(results))
	rows := []giode.Element{}
	for i, r := range results {
		down[i] = r.DownloadMbps
	}
	for i := len(results) - 1; i >= 0 && len(rows) < 12; i-- {
		r := results[i]
		tag := ""
		if r.Scheduled {
			tag = tr("scheduled")
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: 12, Width: -1, Padding: giode.SymmetricInset(2, 6), Align: giode.AlignCenter},
			giode.Text(r.When.Format("Mon Jan 2 15:04"), giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FlexGrow: 1}),
			giode.Text(tag, styleMuted),
			ratePair(fmt.Sprintf("%.1f", r.DownloadMbps), fmt.Sprintf("%.1f", r.UploadMbps), 12),
			giode.Text(fmt.Sprintf("%.0f ms", r.LatencyMs), styleMuted),
		))
	}
	return card(tr("History"), giode.Text(tr("Download, last 30 days"), styleMuted),
		sparkline(down, -1, 90, colDown), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
}
