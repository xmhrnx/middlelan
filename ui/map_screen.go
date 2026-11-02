package ui

import (
	"math"
	"sort"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/xmhrnx/middlelan/internal/geo"
)

// mapScreen shows where in the world the computer's traffic goes.
type mapScreen struct {
	shell  *Shell
	scroll *scroll.Scroll
	scope  *segmented.Control
}

func newMapScreen(sh *Shell) *mapScreen {
	m := &mapScreen{shell: sh}
	m.scope = giode.Input.Segmented("Live", "This session")
	m.scroll = giode.Scroll(m.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return m
}

func (m *mapScreen) view() giode.Element { return m.scroll }

// country is traffic summed per country.
type country struct {
	name, cc string
	bytes    uint64
	conns    int
	owners   map[string]bool
}

func (m *mapScreen) content() giode.Element {
	snap := m.shell.svc.Collector.Snapshot()
	dests := groupDestinations(snap.Apps, m.shell.svc.Resolver, m.scope.Selected() == 0)
	var peak float64
	for _, d := range dests {
		peak = math.Max(peak, float64(d.Bytes)+float64(d.Conns)*1000)
	}
	markers := []mapMarker{}
	countries := map[string]*country{}
	located, public := 0, 0
	for _, d := range dests {
		if d.Scope != geo.ScopePublic {
			continue
		}
		public++
		if d.Located {
			located++
			w := 0.0
			if peak > 0 {
				w = math.Log1p(float64(d.Bytes)+float64(d.Conns)*1000) / math.Log1p(peak)
			}
			c := colDown
			if len(d.Apps) > 2 {
				c = colUp
			}
			markers = append(markers, mapMarker{lat: d.Lat, lon: d.Lon, weight: w, color: c})
		}
		key := d.Country
		if key == "" {
			key = "Unknown"
		}
		c := countries[key]
		if c == nil {
			c = &country{name: key, cc: d.CC, owners: map[string]bool{}}
			countries[key] = c
		}
		c.bytes += d.Bytes
		c.conns += d.Conns
		c.owners[d.Owner] = true
	}
	var home *mapMarker
	if h, ok := m.shell.home(); ok {
		home = &mapMarker{lat: h.Lat, lon: h.Lon}
	}
	subtitle := plural(public, "remote network", "remote networks")
	if !m.shell.svc.Settings.Get().OnlineLookups {
		subtitle += " · locations are off in Settings"
	} else if located < public {
		subtitle += " · locating " + itoa(public-located) + "…"
	}
	return pageBody(
		pageHeader("Map", subtitle, m.scope),
		cardStyled(giode.Styles{Padding: giode.UniformInset(14)}, worldMap(markers, home),
			giode.Row(giode.Styles{Gap: 16, Align: giode.AlignCenter},
				legendDot("You", "theme.success"), legendDot("Destination", colDown), legendDot("Shared by 3+ apps", colUp))),
		responsiveGrid(380, 2, 18, m.countryList(countries), m.ownerList(dests)),
	)
}

func (m *mapScreen) countryList(countries map[string]*country) giode.Element {
	list := make([]*country, 0, len(countries))
	var total uint64
	for _, c := range countries {
		list = append(list, c)
		total += c.bytes
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].bytes != list[j].bytes {
			return list[i].bytes > list[j].bytes
		}
		return list[i].conns > list[j].conns
	})
	rows := []giode.Element{}
	for i, c := range list {
		if i == 10 {
			break
		}
		share := 0.0
		if total > 0 {
			share = float64(c.bytes) / float64(total)
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(2, 6), Width: -1, Wrap: giode.WrapNoWrap},
			countryBadge(c.cc, geo.ScopePublic),
			giode.Stack(giode.Styles{Gap: 5, FlexGrow: 1, Width: -1},
				giode.Row(giode.Styles{Wrap: giode.WrapNoWrap, Width: -1},
					giode.Text(c.name, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium, FlexGrow: 1}),
					giode.Text(formatBytes(c.bytes), styleMuted),
				),
				shareBar(share),
				giode.Text(plural(len(c.owners), "network", "networks")+" · "+plural(c.conns, "connection", "connections"), giode.Styles{Color: "theme.muted", FontSize: sizeTiny}),
			),
		))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyState("globe", "No countries yet", "Countries appear as apps reach the internet.", nil))
	}
	return card("Countries", nil, giode.Stack(giode.Styles{Gap: 4, Width: -1}, rows...))
}

func (m *mapScreen) ownerList(dests []*destination) giode.Element {
	rows := []giode.Element{}
	for _, d := range dests {
		if len(rows) == 10 {
			break
		}
		rows = append(rows, destinationRow(d))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyState("server", "No networks yet", "Companies and networks your apps reach appear here.", nil))
	}
	return card("Networks", nil, giode.Stack(giode.Styles{Gap: 2, Width: -1}, rows...))
}
