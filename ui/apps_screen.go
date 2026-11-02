package ui

import (
	"sort"
	"strings"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// appsScreen lists every program that uses the network.
type appsScreen struct {
	shell  *Shell
	search *input.Input
	filter *segmented.Control
	sortBy *dropdown.Select
	list   *giode.LazyListElement
	conns  *giode.ButtonElement
	rows   []string
}

const (
	filterActive = iota
	filterAll
	filterListening
)

func newAppsScreen(sh *Shell) *appsScreen {
	a := &appsScreen{shell: sh}
	a.search = searchField(msg("Search apps"))
	a.filter = segmentedT("Active", "All", "Listening")
	a.sortBy = selectT(styleField.With(giode.Styles{Width: 150}), "Busiest now", "Most today", "Name")
	a.conns = buttonT("All connections", styleOutline).Variant(giode.VariantOutlined).
		Icon(giode.Icon("waypoints", giode.Styles{Width: 16})).OnClick(func() { sh.navigate(screenConnections) })
	a.list = giode.LazyList(
		func() int { return len(a.rows) },
		func(i int) any { return a.rows[i] },
		func(i int) giode.Element { return sh.appPressable("apps", a.rows[i], -1) },
		giode.Styles{Gap: 2, Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}},
	)
	return a
}

func (a *appsScreen) view() giode.Element {
	snap := a.shell.svc.Collector.Snapshot()
	apps := a.filtered(snap)
	a.rows = a.rows[:0]
	for _, app := range apps {
		a.rows = append(a.rows, app.Name)
	}
	var body giode.Element = a.list
	if len(a.rows) == 0 {
		body = emptyState("app-window", tr("No apps match"), tr("Try another filter, or wait for apps to use the network."), nil)
	}
	return pageBodyFill(
		pageHeader(tr("Apps"), trn("{count} programs use the network", len(snap.Apps)), a.conns),
		toolbar(a.search, a.filter, a.sortBy),
		cardStyled(giode.Styles{FlexGrow: 1, Height: -1, Padding: giode.UniformInset(8)}, body),
	)
}

func (a *appsScreen) filtered(s *monitor.Snapshot) []monitor.App {
	q := strings.ToLower(strings.TrimSpace(a.search.Text()))
	out := make([]monitor.App, 0, len(s.Apps))
	for _, app := range s.Apps {
		switch a.filter.Selected() {
		case filterActive:
			if app.Active == 0 && app.RateRx+app.RateTx == 0 {
				continue
			}
		case filterListening:
			if app.Listening == 0 {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToLower(app.Name+" "+app.Exe), q) {
			continue
		}
		out = append(out, app)
	}
	switch a.sortBy.Selected() {
	case 1:
		sort.SliceStable(out, func(i, j int) bool { return out[i].TodayRx+out[i].TodayTx > out[j].TodayRx+out[j].TodayTx })
	case 2:
		sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	}
	return out
}
