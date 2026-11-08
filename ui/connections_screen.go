package ui

import (
	"sort"
	"strings"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/input"
	"github.com/xmhrnx/giode/components/segmented"
	"github.com/xmhrnx/middlelan/internal/geo"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// connectionsScreen lists every open socket on the computer.
type connectionsScreen struct {
	shell  *Shell
	search *input.Input
	filter *segmented.Control
	list   *giode.LazyListElement
	rows   []monitor.Conn
}

func newConnectionsScreen(sh *Shell) *connectionsScreen {
	c := &connectionsScreen{shell: sh}
	c.search = searchField(msg("Search by app, address, host or port"))
	c.filter = segmentedT("Internet", "Local", "Listening", "All")
	c.list = giode.LazyList(
		func() int { return len(c.rows) },
		func(i int) any { return c.rows[i].Key },
		func(i int) giode.Element { return c.row(c.rows[i]) },
		giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}},
	)
	return c
}

func (c *connectionsScreen) view() giode.Element {
	snap := c.shell.svc.Collector.Snapshot()
	c.rows = c.filtered(snap.Conns)
	var body giode.Element = c.list
	if len(c.rows) == 0 {
		body = emptyState("waypoints", tr("No connections match"), tr("Change the filter or search."), nil)
	}
	return pageBodyFill(
		pageHeader(tr("Connections"), trn("{count} open sockets right now", len(snap.Conns))),
		toolbar(c.search, c.filter),
		cardStyled(giode.Styles{FlexGrow: 1, Height: -1, Padding: giode.UniformInset(8)}, body),
	)
}

func (c *connectionsScreen) filtered(conns []monitor.Conn) []monitor.Conn {
	q := strings.ToLower(strings.TrimSpace(c.search.Text()))
	out := make([]monitor.Conn, 0, len(conns))
	for _, conn := range conns {
		scope := geo.Classify(conn.RemoteIP)
		switch c.filter.Selected() {
		case 0:
			if conn.Listening() || scope != geo.ScopePublic {
				continue
			}
		case 1:
			if conn.Listening() || scope == geo.ScopePublic {
				continue
			}
		case 2:
			if !conn.Listening() {
				continue
			}
		}
		if q != "" {
			info, _ := c.shell.svc.Resolver.Peek(conn.RemoteIP)
			hay := strings.ToLower(strings.Join([]string{conn.Process, conn.RemoteIP, conn.LocalIP, itoa(conn.RemotePort), itoa(conn.LocalPort), info.Domain, info.Host, info.Owner(), info.Country}, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, conn)
	}
	sort.SliceStable(out, func(i, j int) bool {
		bi, bj := out[i].Sent+out[i].Recv, out[j].Sent+out[j].Recv
		if bi != bj {
			return bi > bj
		}
		return out[i].Process < out[j].Process
	})
	return out
}

func (c *connectionsScreen) row(conn monitor.Conn) giode.Element {
	var where giode.Element
	if conn.Listening() {
		where = giode.Text(tr("Listening on {address}", "address", endpointLabel(conn.LocalIP, conn.LocalPort)), giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontFamily: fontMono})
	} else {
		info := c.shell.svc.Resolver.Get(conn.RemoteIP)
		name := info.Domain
		if name == "" {
			name = nonEmpty(info.Host, info.Owner())
		}
		place := placeName(info.Place())
		parts := []string{endpointLabel(conn.RemoteIP, conn.RemotePort)}
		if name != "" {
			parts = append(parts, name)
		}
		if place != "" {
			parts = append(parts, place)
		}
		where = giode.Text(strings.Join(parts, " · "), giode.Styles{Color: "theme.text", FontSize: sizeSmall, TextOverflow: giode.TextOverflowEllipsis})
	}
	right := []giode.Element{giode.Text(strings.ToUpper(conn.Proto), styleLabel)}
	if conn.HasBytes && conn.Sent+conn.Recv > 0 {
		right = append(right, giode.Text(formatBytes(conn.Sent+conn.Recv), giode.Styles{Color: "theme.text", FontSize: sizeSmall}))
	}
	state := conn.State
	if state == "" {
		state = "—"
	}
	row := giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(10, 8), Width: -1, Wrap: giode.WrapNoWrap},
		appAvatar(conn.Process, 28),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
				giode.Text(conn.Process, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightSemiBold, TextOverflow: giode.TextOverflowEllipsis}),
				giode.Text(state, giode.Styles{Color: "theme.muted", FontSize: sizeTiny}),
			),
			where,
		),
		giode.Stack(giode.Styles{Align: giode.AlignEnd, Gap: 3}, right...),
	)
	if conn.Listening() || conn.RemoteIP == "" {
		return withMenu(row, c.shell.appMenu(conn.Process))
	}
	info := c.shell.svc.Resolver.Get(conn.RemoteIP)
	return withMenu(row, c.shell.hostMenu(conn.RemoteIP, nonEmpty(info.Domain, info.Host)))
}

// showOnly shows one program's connections, from its menu.
func (c *connectionsScreen) showOnly(app string) {
	c.search.SetText(app)
	c.filter.SetSelected(3)
}
