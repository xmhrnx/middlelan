package ui

import (
	"sort"
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/copybutton"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/xmhrnx/middlelan/internal/firewall"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// appDetailScreen shows one program: its traffic, and every place it
// connects to, now and before.
type appDetailScreen struct {
	shell  *Shell
	name   string
	scroll *scroll.Scroll
	back   *giode.ButtonElement
	copy   *copybutton.CopyButton
	block  *giode.ButtonElement
}

func newAppDetailScreen(sh *Shell) *appDetailScreen {
	d := &appDetailScreen{shell: sh}
	d.scroll = giode.Scroll(d.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	d.back = backButton("Apps", func() { sh.navigate(screenApps) })
	d.block = buttonT("Block", styleOutline.With(giode.Styles{Color: "theme.danger", BorderColor: colDangerSoft, Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})).
		Variant(giode.VariantOutlined).Icon(giode.Icon("ban", giode.Styles{Width: 14})).OnClick(d.toggleBlock)
	d.copy = giode.CopyButton(func() string {
		a, _ := sh.findApp(d.name)
		return a.Exe
	})
	return d
}

func (d *appDetailScreen) show(name string) {
	d.name = name
	d.scroll.ScrollTo(0)
}

func (d *appDetailScreen) view() giode.Element { return d.scroll }

func (d *appDetailScreen) content() giode.Element {
	a, ok := d.shell.findApp(d.name)
	if !ok {
		a = monitor.App{Name: d.name}
	}
	now := time.Now()
	store := d.shell.svc.Collector.Store()
	return pageBody(
		giode.Row(giode.Styles{}, d.back),
		d.header(a),
		responsiveGrid(170, 4, 14,
			statTile("gauge", tr("Right now"), rateOrDash(a), "↓ "+formatRate(a.RateRx)+"  ↑ "+formatRate(a.RateTx), colDown),
			statTile("activity", tr("Today"), formatBytes(a.TodayRx+a.TodayTx), formatBytes(a.TodayRx)+" down · "+formatBytes(a.TodayTx)+" up", "theme.success"),
			statTile("clock", tr("This session"), formatBytes(a.SessionRx+a.SessionTx), "Since MiddleLAN started", colUp),
			statTile("waypoints", tr("Connections"), itoa(a.Active), plural(a.Listening, "listening port", "listening ports"), "theme.warning"),
		),
		responsiveGrid(380, 2, 18,
			card(tr("Activity"), nil, sparkline(a.History, -1, 110, colDown), giode.Text("Last "+itoa(len(a.History))+" samples", styleMuted)),
			card(tr("Last 7 days"), nil, usageBars(store.Daily(a.Name, 7, now), 110)),
		),
		d.liveEndpoints(a, now),
		d.history(a, now),
		d.listening(a),
	)
}

func rateOrDash(a monitor.App) string {
	if !a.KnownBytes {
		return "—"
	}
	return formatRate(a.RateRx + a.RateTx)
}

func (d *appDetailScreen) header(a monitor.App) giode.Element {
	sub := a.Exe
	if sub == "" {
		sub = tr("Executable path unavailable")
	}
	meta := []string{}
	if len(a.PIDs) > 0 {
		ids := make([]string, len(a.PIDs))
		for i, p := range a.PIDs {
			ids[i] = itoa(p)
		}
		meta = append(meta, "PID "+strings.Join(ids, ", "))
	}
	if !a.FirstSeen.IsZero() {
		meta = append(meta, tr("First seen {date}", "date", a.FirstSeen.Format("Jan 2, 2006")))
	}
	status := pill(tr("Idle"), "theme.muted")
	if a.Active > 0 {
		status = pill(tr("Online"), "theme.success")
	}
	right := []giode.Element{status}
	if a.Exe != "" {
		right = append(right, d.copy)
	}
	if fw := d.shell.svc.Firewall; fw != nil && fw.Capabilities().PerApp {
		if d.blockRule(a.Name) != "" {
			d.block.Label(tr("Unblock"))
		} else {
			d.block.Label(tr("Block"))
		}
		right = append(right, d.block)
	}
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 16, Width: -1, Wrap: giode.WrapNoWrap},
		giode.Avatar(a.Name, giode.Styles{Width: 56}),
		giode.Stack(giode.Styles{Gap: 4, FlexGrow: 1, Width: -1},
			giode.Text(a.Name, styleTitle),
			giode.Text(sub, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FontFamily: fontMono, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(strings.Join(meta, " · "), styleMuted),
		),
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, right...),
	)
}

func (d *appDetailScreen) liveEndpoints(a monitor.App, now time.Time) giode.Element {
	rows := []giode.Element{}
	for i, e := range a.Endpoints {
		if i == 60 {
			break
		}
		info := d.shell.svc.Resolver.Get(e.IP)
		rows = append(rows, endpointRow(info, e.Port, e.Proto, e.Rx+e.Tx, e.Active, e.LastSeen, now))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyState("globe", tr("No connections seen yet"), tr("Places this app connects to appear here while MiddleLAN runs."), nil))
	}
	return card(tr("Where it connects"), giode.Text(trn("{count} endpoints this session", len(a.Endpoints)), styleMuted),
		giode.Stack(giode.Styles{Gap: 0, Width: -1}, withDividers(rows)...))
}

func (d *appDetailScreen) history(a monitor.App, now time.Time) giode.Element {
	dests := d.shell.svc.Collector.Store().Destinations(a.Name)
	if len(dests) == 0 {
		return nil
	}
	sort.SliceStable(dests, func(i, j int) bool {
		return dests[i].Count > dests[j].Count && dests[i].Rx+dests[i].Tx == dests[j].Rx+dests[j].Tx
	})
	rows := []giode.Element{}
	for i, dst := range dests {
		if i == 25 {
			break
		}
		info := d.shell.svc.Resolver.Get(dst.IP)
		rows = append(rows, endpointRow(info, dst.Port, "tcp", dst.Rx+dst.Tx, false, dst.LastSeen, now))
	}
	return card(tr("Known destinations"), giode.Text(tr("Remembered across sessions"), styleMuted),
		giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
}

func (d *appDetailScreen) listening(a monitor.App) giode.Element {
	rows := []giode.Element{}
	for _, c := range a.Conns {
		if !c.Listening() {
			continue
		}
		scope := tr("Reachable from the network")
		if c.LocalIP == "127.0.0.1" || c.LocalIP == "::1" {
			scope = tr("This device only")
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(4, 6), Width: -1},
			iconBadge("ear", "theme.warning", 28),
			giode.Text(strings.ToUpper(c.Proto)+" "+endpointLabel(c.LocalIP, c.LocalPort), giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontFamily: fontMono, FlexGrow: 1}),
			giode.Text(serviceName(c.LocalPort), styleMuted),
			giode.Text(scope, styleMuted),
		))
	}
	if len(rows) == 0 {
		return nil
	}
	return card(tr("Listening ports"), nil, giode.Stack(giode.Styles{Width: -1}, rows...))
}

// serviceName is a port's well-known name, or "".
func serviceName(port int) string { return wellKnown[port] }

// withDividers separates rows with hairlines.
func withDividers(rows []giode.Element) []giode.Element {
	out := make([]giode.Element, 0, len(rows)*2)
	for i, r := range rows {
		if i > 0 {
			out = append(out, giode.Divider(giode.Styles{Color: "theme.border"}))
		}
		out = append(out, r)
	}
	return out
}

// blockRule returns the ID of the rule blocking the whole app, or "".
func (d *appDetailScreen) blockRule(name string) string {
	for _, r := range d.shell.svc.Firewall.Rules() {
		if r.App == name && r.Dest == "" {
			return r.ID
		}
	}
	return ""
}

func (d *appDetailScreen) toggleBlock() {
	a, _ := d.shell.findApp(d.name)
	if id := d.blockRule(d.name); id != "" {
		d.shell.firewallChange(func(fw *firewall.Firewall) error { return fw.Remove(id) })
		return
	}
	d.shell.blockRule(firewall.Rule{App: d.name, Exe: a.Exe, Note: tr("Blocked from the app's page")})
}
