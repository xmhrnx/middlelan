package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// discoveryTool lists UPnP devices and Bonjour (mDNS) services.
type discoveryTool struct {
	shell    *Shell
	run      *giode.ButtonElement
	runner   toolRunner
	upnp     []nettools.UPnPDevice
	services []nettools.MDNSService
	errs     []string
	done     bool
}

func newDiscoveryTool(sh *Shell) *discoveryTool {
	d := &discoveryTool{shell: sh, runner: toolRunner{shell: sh}}
	d.run = runButton("Discover", d.start, &d.runner)
	return d
}

func (d *discoveryTool) setTarget(string) {}

func (d *discoveryTool) start() {
	d.upnp, d.services, d.errs, d.done = nil, nil, nil, false
	d.runner.start(func(ctx context.Context) {
		type res struct {
			upnp []nettools.UPnPDevice
			mdns []nettools.MDNSService
			err  error
		}
		ch := make(chan res, 2)
		go func() { u, err := nettools.DiscoverUPnP(ctx, 4*time.Second); ch <- res{upnp: u, err: err} }()
		go func() { m, err := nettools.BrowseMDNS(ctx, 4*time.Second); ch <- res{mdns: m, err: err} }()
		for range 2 {
			r := <-ch
			d.runner.post(func() {
				if r.upnp != nil {
					d.upnp = r.upnp
				}
				if r.mdns != nil {
					d.services = r.mdns
				}
				if r.err != nil && ctx.Err() == nil {
					d.errs = append(d.errs, r.err.Error())
				}
			})
		}
		d.runner.post(func() { d.done = true })
	})
}

func (d *discoveryTool) view(t *toolsScreen, info toolInfo) giode.Element {
	form := giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1},
		giode.Text("Listens for smart TVs, speakers, printers, consoles and media servers announcing themselves.", giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FlexGrow: 1}),
		syncRunLabel(d.run, "Discover", &d.runner))
	results := []giode.Element{}
	for _, e := range d.errs {
		results = append(results, noticeBanner("info", e, "theme.warning", nil))
	}
	if d.runner.running {
		results = append(results, giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter}, giode.Spinner(giode.Styles{Width: 16, Color: "theme.primary"}), giode.Text("Listening for announcements…", styleMuted)))
	}
	if len(d.upnp) > 0 {
		rows := []giode.Element{}
		for _, u := range d.upnp {
			name := nonEmpty(u.FriendlyName, u.Server)
			sub := strings.Trim(strings.Join([]string{u.Manufacturer, u.ModelName, u.IP}, " · "), " ·")
			rows = append(rows, discoveryRow("cast", name, sub))
		}
		results = append(results, card("UPnP devices", giode.Text(itoa(len(d.upnp)), styleMuted), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...)))
	}
	if len(d.services) > 0 {
		sort.Slice(d.services, func(i, j int) bool { return d.services[i].Service < d.services[j].Service })
		rows := []giode.Element{}
		for _, s := range d.services {
			sub := s.Service
			if s.IP != "" {
				sub += " · " + endpointLabel(s.IP, s.Port)
			}
			rows = append(rows, discoveryRow("radio", nonEmpty(s.Instance, s.Host), sub))
		}
		results = append(results, card("Bonjour services", giode.Text(itoa(len(d.services)), styleMuted), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...)))
	}
	if d.done && len(d.upnp) == 0 && len(d.services) == 0 {
		results = append(results, emptyState("cast", "Nothing announced itself", "Devices may be asleep, or the network may block multicast.", nil))
	}
	return t.toolFrame(info, form, results...)
}

func discoveryRow(icon, title, sub string) giode.Element {
	return giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Padding: giode.SymmetricInset(2, 7), Wrap: giode.WrapNoWrap},
		iconBadge(icon, "theme.accent", 30),
		giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1, Width: -1},
			giode.Text(title, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(sub, giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
		))
}
