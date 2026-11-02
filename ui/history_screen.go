package ui

import (
	"fmt"
	"sort"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/scroll"
	"github.com/xmhrnx/giode/components/segmented"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// historyScreen shows usage over time, by hour or day, and which apps
// used the network in any period ("what used data at 3 a.m.?").
type historyScreen struct {
	shell    *Shell
	scroll   *scroll.Scroll
	span     *segmented.Control
	chart    *giode.PressableElement
	selected int
	chartW   float32
	// period being shown, rebuilt each frame
	periods []time.Time
	series  barSeries
}

// historySpans are the ranges the page offers.
var historySpans = []struct {
	label  string
	hours  int // hourly bars; 0 means daily
	days   int
	format string
}{
	{msg("24 hours"), 24, 0, "15:00"},
	{msg("7 days"), 0, 7, "Mon 2"},
	{msg("30 days"), 0, 30, "Jan 2"},
	{msg("90 days"), 0, 90, "Jan 2"},
}

func newHistoryScreen(sh *Shell) *historyScreen {
	h := &historyScreen{shell: sh, selected: -1}
	labels := make([]string, len(historySpans))
	for i, s := range historySpans {
		labels[i] = s.label
	}
	h.span = segmentedT(labels...).OnChange(func(int) { h.selected = -1 })
	h.chart = giode.Pressable(func(giode.PressState) giode.Element {
		return giode.Raw(func(gtx layoutContext) layoutDimensions {
			h.chartW = float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
			return stackedBars(h.series, 180).Layout(gtx)
		})
	}).Label(tr("Usage chart")).OnClick(func(e giode.ClickEvent) {
		i := barAt(e.PositionDp.X, h.chartW, len(h.periods))
		if i == h.selected {
			i = -1
		}
		h.selected = i
	})
	h.scroll = giode.Scroll(h.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return h
}

func (h *historyScreen) view() giode.Element { return h.scroll }

func (h *historyScreen) content() giode.Element {
	store := h.shell.svc.Collector.Store()
	now := time.Now()
	span := historySpans[h.span.Selected()]
	h.periods = h.periods[:0]
	h.series = barSeries{selected: h.selected}
	var total monitor.ByteCount
	if span.hours > 0 {
		for _, u := range store.Hourly("", span.hours, now) {
			h.periods = append(h.periods, u.Hour)
			h.series.rx, h.series.tx = append(h.series.rx, u.Rx), append(h.series.tx, u.Tx)
			total.Rx, total.Tx = total.Rx+u.Rx, total.Tx+u.Tx
		}
	} else {
		for _, u := range store.Daily("", span.days, now) {
			t, _ := time.ParseInLocation("2006-01-02", u.Day, time.Local)
			h.periods = append(h.periods, t)
			h.series.rx, h.series.tx = append(h.series.rx, u.Rx), append(h.series.tx, u.Tx)
			total.Rx, total.Tx = total.Rx+u.Rx, total.Tx+u.Tx
		}
	}
	if h.selected >= len(h.periods) {
		h.selected = -1
	}
	axis := []giode.Element{}
	if n := len(h.periods); n > 0 {
		for _, i := range []int{0, n / 2, n - 1} {
			axis = append(axis, giode.Text(h.periods[i].Format(span.format), styleMuted))
		}
	}
	chart := card(tr("Usage"), ratePair(formatBytes(total.Rx), formatBytes(total.Tx), 12),
		h.chart,
		giode.Row(giode.Styles{Justify: giode.JustifySpaceBetween, Width: -1}, axis...),
		giode.Text(tr("Click a bar to see which apps used the network then."), styleMuted),
	)
	return pageBody(
		pageHeader(tr("History"), tr("Usage is kept on this device: hourly for 14 days, daily for 90"), h.span),
		chart,
		h.breakdown(span.hours > 0),
		h.comparison(now),
	)
}

// breakdown lists the apps in the selected period, or in the whole span.
func (h *historyScreen) breakdown(hourly bool) giode.Element {
	store := h.shell.svc.Collector.Store()
	title := tr("Apps in this period")
	apps := map[string]monitor.ByteCount{}
	if h.selected >= 0 {
		t := h.periods[h.selected]
		if hourly {
			apps = store.HourApps(t)
			title = tr("Apps from {start} to {end}", "start", weekdayShort(t)+" "+t.Format("15:00"), "end", t.Add(time.Hour).Format("15:00"))
		} else {
			apps = store.Day(t)
			title = tr("Apps on {date}", "date", weekdayShort(t)+" "+t.Format("2006-01-02"))
		}
	} else {
		for _, t := range h.periods {
			var part map[string]monitor.ByteCount
			if hourly {
				part = store.HourApps(t)
			} else {
				part = store.Day(t)
			}
			for a, c := range part {
				v := apps[a]
				v.Rx, v.Tx = v.Rx+c.Rx, v.Tx+c.Tx
				apps[a] = v
			}
		}
	}
	type row struct {
		name string
		c    monitor.ByteCount
	}
	rows := make([]row, 0, len(apps))
	var total uint64
	for a, c := range apps {
		if c.Rx+c.Tx == 0 {
			continue
		}
		rows = append(rows, row{a, c})
		total += c.Rx + c.Tx
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].c.Rx+rows[i].c.Tx > rows[j].c.Rx+rows[j].c.Tx })
	els := []giode.Element{}
	for i, r := range rows {
		if i == 15 {
			break
		}
		share := float64(r.c.Rx+r.c.Tx) / float64(max(total, 1))
		els = append(els, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Padding: giode.SymmetricInset(2, 6), Wrap: giode.WrapNoWrap},
			giode.Avatar(r.name, giode.Styles{Width: 30}),
			giode.Stack(giode.Styles{Gap: 4, FlexGrow: 1, Width: -1},
				giode.Row(giode.Styles{Width: -1, Wrap: giode.WrapNoWrap},
					giode.Text(r.name, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium, FlexGrow: 1}),
					giode.Text(fmt.Sprintf("%.0f%%", share*100), styleMuted)),
				shareBar(share)),
			giode.Box(giode.Styles{MinWidth: 150}, ratePair(formatBytes(r.c.Rx), formatBytes(r.c.Tx), 11)),
		))
	}
	if len(els) == 0 {
		els = append(els, emptyState("clock-arrow-up", tr("Nothing used the network"), tr("No traffic was recorded in this period."), nil))
	}
	return card(title, nil, giode.Stack(giode.Styles{Gap: 2, Width: -1}, els...))
}

// comparison puts this week next to last week.
func (h *historyScreen) comparison(now time.Time) giode.Element {
	days := h.shell.svc.Collector.Store().Daily("", 14, now)
	var last, this uint64
	for i, d := range days {
		if i < 7 {
			last += d.Rx + d.Tx
		} else {
			this += d.Rx + d.Tx
		}
	}
	change := tr("No data from last week yet")
	tone := giode.Color("theme.muted")
	if last > 0 {
		pct := (float64(this) - float64(last)) / float64(last) * 100
		change = tr("{percent} vs the 7 days before", "percent", fmt.Sprintf("%+.0f%%", pct))
		tone = "theme.success"
		if pct > 0 {
			tone = "theme.warning"
		}
	}
	return responsiveGrid(220, 3, 14,
		statTile("activity", tr("Last 7 days"), formatBytes(this), change, tone),
		statTile("clock", tr("The 7 days before"), formatBytes(last), "", "theme.info"),
		statTile("gauge", tr("Daily average"), formatBytes(this/7), tr("This week"), colDown),
	)
}
