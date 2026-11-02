package ui

import (
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/scroll"
)

// toolID identifies a network tool.
type toolID int

const (
	toolNone toolID = iota
	toolSpeed
	toolPing
	toolTrace
	toolPorts
	toolDNS
	toolWhois
	toolWake
	toolNetInfo
	toolDiscovery
	toolRouter
	toolTLS
	toolSubnet
	toolUptime
	toolWifi
)

// toolInfo describes a tool in the catalog.
type toolInfo struct {
	id    toolID
	name  string
	desc  string
	icon  string
	color giode.Color
}

var toolCatalog = []toolInfo{
	{toolSpeed, msg("Speed test"), msg("Download, upload, latency and jitter of your internet connection."), "gauge", "theme.primary"},
	{toolUptime, msg("Internet monitor"), msg("Watches latency and packet loss continuously and logs outages."), "signal", "theme.success"},
	{toolRouter, msg("Router check"), msg("Audits your router for risky open services and DNS hijacking."), "shield-check", "theme.warning"},
	{toolWifi, msg("Wi-Fi analyzer"), msg("Signal strength, nearby networks, crowded channels and weak security."), "wifi", "theme.primary"},
	{toolDiscovery, msg("Service discovery"), msg("Finds UPnP and Bonjour devices and services on the network."), "cast", "theme.accent"},
	{toolPing, msg("Ping"), msg("Measures round-trip time and packet loss to any host."), "activity", "theme.info"},
	{toolTrace, msg("Traceroute"), msg("Shows each router between you and a host."), "route", "theme.accent"},
	{toolPorts, msg("Port scanner"), msg("Finds open TCP ports and the services behind them."), "scan-search", "theme.danger"},
	{toolDNS, msg("DNS lookup"), msg("Queries A, AAAA, MX, TXT, NS and other records."), "search", "theme.primary"},
	{toolWhois, msg("WHOIS"), msg("Who owns a domain name or IP address."), "book-open", "theme.info"},
	{toolTLS, msg("Certificate inspector"), msg("Checks a site's TLS certificate, chain and expiry."), "lock", "theme.success"},
	{toolWake, msg("Wake on LAN"), msg("Turns on a sleeping computer over the network."), "power", "theme.warning"},
	{toolNetInfo, msg("Network info"), msg("Interfaces, addresses, router, DNS servers and public IP."), "network", "theme.accent"},
	{toolSubnet, msg("Subnet calculator"), msg("Network, broadcast, host range and mask of any CIDR."), "calculator", "theme.info"},
}

// tool is one tool's page.
type tool interface {
	view(t *toolsScreen, info toolInfo) giode.Element
	// setTarget prefills the tool's host field.
	setTarget(host string)
}

// toolsScreen is the toolbox: a catalog, or the open tool.
type toolsScreen struct {
	shell  *Shell
	active toolID
	scroll *scroll.Scroll
	back   *giode.ButtonElement
	tools  map[toolID]tool
	cards  map[toolID]*giode.PressableElement
}

func newToolsScreen(sh *Shell) *toolsScreen {
	t := &toolsScreen{shell: sh, cards: map[toolID]*giode.PressableElement{}}
	t.back = backButton("Tools", func() { t.active = toolNone })
	t.tools = map[toolID]tool{
		toolSpeed: newSpeedTool(sh), toolPing: newPingTool(sh), toolTrace: newTraceTool(sh), toolPorts: newPortsTool(sh),
		toolDNS: newDNSTool(sh), toolWhois: newWhoisTool(sh), toolWake: newWakeTool(sh), toolNetInfo: newNetInfoTool(sh),
		toolDiscovery: newDiscoveryTool(sh), toolRouter: newRouterTool(sh), toolTLS: newTLSTool(sh),
		toolSubnet: newSubnetTool(sh), toolUptime: newUptimeTool(sh), toolWifi: newWifiTool(sh),
	}
	for _, info := range toolCatalog {
		info := info
		card := giode.Pressable(func(s giode.PressState) giode.Element {
			return toolCard(info, s)
		}).OnClick(func(giode.ClickEvent) { t.open(info.id) })
		localized(func() { card.Label(tr(info.name)) })
		t.cards[info.id] = card
	}
	t.scroll = giode.Scroll(t.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return t
}

func (t *toolsScreen) open(id toolID) {
	t.active = id
	t.scroll.ScrollTo(0)
}

// openWith opens a tool with its host prefilled, from anywhere in the app.
func (t *toolsScreen) openWith(id toolID, host string) {
	if tl, ok := t.tools[id]; ok {
		tl.setTarget(host)
	}
	t.open(id)
	t.shell.navigate(screenTools)
}

func (t *toolsScreen) view() giode.Element { return t.scroll }

func (t *toolsScreen) content() giode.Element {
	if t.active != toolNone {
		giode.HandleBack(func() { t.active = toolNone })
		for _, info := range toolCatalog {
			if info.id == t.active {
				return t.tools[info.id].view(t, info)
			}
		}
	}
	cards := make([]giode.Element, 0, len(toolCatalog))
	for _, info := range toolCatalog {
		cards = append(cards, t.cards[info.id])
	}
	return pageBody(
		pageHeader(tr("Tools"), tr("Everything you need to diagnose and secure your network")),
		responsiveGrid(250, 3, 14, cards...),
	)
}

// toolCard is a tool in the catalog.
func toolCard(info toolInfo, s giode.PressState) giode.Element {
	border := giode.Color("theme.border")
	if s.Hovered || s.Pressed {
		border = giode.Mix(info.color, "theme.surface", 0.5)
	}
	return giode.Box(styleCard.With(giode.Styles{BorderColor: border, Gap: 10, Height: 150, Width: -1}),
		iconBadge(info.icon, info.color, 40),
		giode.Text(tr(info.name), giode.Styles{Color: "theme.text", FontSize: 15, FontWeight: giode.FontWeightSemiBold}),
		giode.Text(tr(info.desc), giode.Styles{Color: "theme.muted", FontSize: sizeSmall, MaxLines: 2}),
	)
}
