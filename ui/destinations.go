package ui

import (
	"sort"

	"github.com/xmhrnx/middlelan/internal/geo"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// destination is traffic to one owner (a company, network or host),
// summed over every app.
type destination struct {
	Owner   string
	Place   string
	Country string
	CC      string
	Apps    map[string]bool
	IPs     map[string]bool
	Bytes   uint64
	Conns   int
	Lat     float64
	Lon     float64
	Located bool
	Scope   geo.Scope
}

// groupDestinations sums the endpoints of the apps by who owns them. Only
// endpoints seen within the snapshot's lifetime are included.
func groupDestinations(apps []monitor.App, res *geo.Resolver, activeOnly bool) []*destination {
	groups := map[string]*destination{}
	for _, a := range apps {
		for _, e := range a.Endpoints {
			if activeOnly && !e.Active {
				continue
			}
			info := res.Get(e.IP)
			if info.Scope == geo.ScopeLoopback || info.Scope == geo.ScopeInvalid {
				continue
			}
			owner := info.Owner()
			if owner == "" {
				owner = info.Host
			}
			if owner == "" {
				owner = e.IP
			}
			if info.Scope != geo.ScopePublic {
				owner = string(info.Scope)
			}
			d := groups[owner]
			if d == nil {
				d = &destination{Owner: owner, Apps: map[string]bool{}, IPs: map[string]bool{}, Scope: info.Scope}
				groups[owner] = d
			}
			if d.Place == "" {
				d.Place, d.Country, d.CC = info.Place(), info.Country, info.CountryCode
			}
			if info.Located && !d.Located {
				d.Lat, d.Lon, d.Located = info.Lat, info.Lon, true
			}
			d.Apps[a.Name] = true
			d.IPs[e.IP] = true
			d.Bytes += e.Rx + e.Tx
			if e.Active {
				d.Conns++
			}
		}
	}
	out := make([]*destination, 0, len(groups))
	for _, d := range groups {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		if out[i].Conns != out[j].Conns {
			return out[i].Conns > out[j].Conns
		}
		return out[i].Owner < out[j].Owner
	})
	return out
}
