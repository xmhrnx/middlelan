package ui

import (
	"context"
	"strings"

	"github.com/mehran9675/giode"
)

// netInfoTool shows this device's network configuration.
type netInfoTool struct {
	shell   *Shell
	refresh *giode.ButtonElement
}

func newNetInfoTool(sh *Shell) *netInfoTool {
	n := &netInfoTool{shell: sh}
	n.refresh = buttonT("Refresh", styleOutline).Variant(giode.VariantOutlined).
		Icon(giode.Icon("refresh-cw", giode.Styles{Width: 14})).OnClick(func() { sh.RefreshNetwork(context.Background()) })
	return n
}

func (n *netInfoTool) setTarget(string) {}

func (n *netInfoTool) view(t *toolsScreen, info toolInfo) giode.Element {
	ni, pub := n.shell.netInfo.Load(), n.shell.public.Load()
	fact := func(label, value string) giode.Element {
		if value == "" {
			value = "—"
		}
		return giode.Row(giode.Styles{Gap: 12, Width: -1, Padding: giode.SymmetricInset(0, 4), Wrap: giode.WrapNoWrap},
			giode.Text(label, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, Width: 130}),
			giode.Text(value, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1}).Selectable("ni-"+label),
		)
	}
	results := []giode.Element{}
	if ni != nil {
		results = append(results, card(tr("This device"), n.refresh,
			fact(tr("Host name"), ni.Hostname), fact(tr("Local IP"), ni.LocalIP), fact(tr("Subnet"), ni.Subnet),
			fact(tr("Router"), ni.Gateway), fact(tr("DNS servers"), strings.Join(ni.DNSServers, ", "))))
	}
	if pub != nil {
		results = append(results, card(tr("Internet"), nil,
			fact(tr("Public IP"), pub.IP), fact(tr("Provider"), nonEmpty(pub.ISP, pub.Org)), fact(tr("ASN"), pub.ASN),
			fact(tr("Location"), strings.Trim(strings.Join([]string{pub.City, pub.Region, pub.Country}, ", "), ", "))))
	} else if !n.shell.svc.Settings.Get().OnlineLookups {
		results = append(results, noticeBanner("info", tr("Public IP lookup is off. Turn on online lookups in Settings to see it."), "theme.info", nil))
	}
	if ni != nil {
		rows := []giode.Element{}
		for _, ifc := range ni.Interfaces {
			st := pill(tr("Down"), "theme.muted")
			if ifc.Up {
				st = pill(tr("Up"), "theme.success")
			}
			rows = append(rows, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Padding: giode.SymmetricInset(2, 6), Wrap: giode.WrapNoWrap},
				giode.Icon("plug", giode.Styles{Width: 18, Color: "theme.muted"}),
				giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1, Width: -1},
					giode.Text(ifc.Name, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium}),
					giode.Text(strings.Join(append([]string{strings.ToUpper(ifc.MAC)}, ifc.Addrs...), " · "), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, FontFamily: fontMono}),
				),
				giode.Text("MTU "+itoa(ifc.MTU), styleMuted),
				st,
			))
		}
		results = append(results, card(tr("Interfaces"), nil, giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...)))
	}
	if len(results) == 0 {
		results = append(results, emptyState("network", tr("Reading network details…"), "", n.refresh))
	}
	return t.toolFrame(info, nil, results...)
}
