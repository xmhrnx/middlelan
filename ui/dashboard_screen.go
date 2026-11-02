package ui

import (
	"fmt"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// dashboardScreen is the overview: live rates, today's totals, the busiest
// apps and where traffic goes.
type dashboardScreen struct {
	shell   *Shell
	scroll  *scroll.Scroll
	allApps *giode.ButtonElement
	allDest *giode.ButtonElement
	notices *limitsNotice
}

func newDashboardScreen(sh *Shell) *dashboardScreen {
	d := &dashboardScreen{shell: sh, notices: newLimitsNotice(sh)}
	d.scroll = giode.Scroll(d.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	d.allApps = giode.Button("All apps", styleOutline).Variant(giode.VariantOutlined).OnClick(func() { sh.navigate(screenApps) })
	d.allDest = giode.Button("Map", styleOutline).Variant(giode.VariantOutlined).OnClick(func() { sh.navigate(screenMap) })
	return d
}

func (d *dashboardScreen) view() giode.Element { return d.scroll }

func (d *dashboardScreen) content() giode.Element {
	snap := d.shell.svc.Collector.Snapshot()
	kids := []giode.Element{
		pageHeader("Overview", "Live network activity on this device"),
	}
	if n := d.notices.view(snap); n != nil {
		kids = append(kids, n)
	}
	kids = append(kids,
		d.tiles(snap),
		card("Live throughput", giode.Text(historySpan(snap), styleMuted), rateChart(snap.History, 190)),
		responsiveGrid(380, 2, 18, d.topApps(snap), d.topDestinations(snap)),
		card("Last 7 days", ratePair(
			formatBytes(sumDays(d.week(), true)), formatBytes(sumDays(d.week(), false)), 12),
			usageBars(d.week(), 140)),
	)
	return pageBody(kids...)
}

func (d *dashboardScreen) week() []monitor.DayUsage {
	return d.shell.svc.Collector.Store().Daily("", 7, time.Now())
}

func sumDays(days []monitor.DayUsage, rx bool) uint64 {
	var t uint64
	for _, d := range days {
		if rx {
			t += d.Rx
		} else {
			t += d.Tx
		}
	}
	return t
}

func historySpan(s *monitor.Snapshot) string {
	if len(s.History) < 2 {
		return "Collecting…"
	}
	span := s.History[len(s.History)-1].Time.Sub(s.History[0].Time).Round(time.Second)
	return "Last " + span.String()
}

func (d *dashboardScreen) tiles(s *monitor.Snapshot) giode.Element {
	peakRx, peakTx := 0.0, 0.0
	for _, p := range s.History {
		peakRx, peakTx = max(peakRx, p.Rx), max(peakTx, p.Tx)
	}
	active, conns := 0, 0
	for _, a := range s.Apps {
		if a.Active > 0 {
			active++
			conns += a.Active
		}
	}
	return responsiveGrid(150, 4, 12,
		statTile("arrow-down", "Download", formatRate(s.RateRx), "Peak "+formatRate(peakRx), colDown),
		statTile("arrow-up", "Upload", formatRate(s.RateTx), "Peak "+formatRate(peakTx), colUp),
		statTile("activity", "Today", formatBytes(s.TodayRx+s.TodayTx),
			fmt.Sprintf("%s down · %s up", formatBytes(s.TodayRx), formatBytes(s.TodayTx)), "theme.success"),
		statTile("app-window", "Apps online", itoa(active), plural(conns, "open connection", "open connections"), "theme.warning"),
	)
}

func (d *dashboardScreen) topApps(s *monitor.Snapshot) giode.Element {
	var total float64
	var totalToday uint64
	for _, a := range s.Apps {
		total += a.RateRx + a.RateTx
		totalToday += a.TodayRx + a.TodayTx
	}
	rows := []giode.Element{}
	for i, a := range s.Apps {
		if i == 6 {
			break
		}
		share := 0.0
		switch {
		case total > 0:
			share = (a.RateRx + a.RateTx) / total
		case totalToday > 0:
			share = float64(a.TodayRx+a.TodayTx) / float64(totalToday)
		}
		rows = append(rows, d.shell.appPressable("dash", a.Name, share))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyState("app-window", "Waiting for traffic", "Apps appear here as soon as they use the network.", nil))
	}
	return card("Top apps", d.allApps, giode.Stack(giode.Styles{Gap: 2, Width: -1}, rows...))
}

func (d *dashboardScreen) topDestinations(s *monitor.Snapshot) giode.Element {
	dests := groupDestinations(s.Apps, d.shell.svc.Resolver, false)
	rows := []giode.Element{}
	for i, dst := range dests {
		if i == 6 {
			break
		}
		rows = append(rows, destinationRow(dst))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyState("globe", "No destinations yet", "Remote hosts show up here with their owner and location.", nil))
	}
	return card("Where traffic goes", d.allDest, giode.Stack(giode.Styles{Gap: 2, Width: -1}, rows...))
}
