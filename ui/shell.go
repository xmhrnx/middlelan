package ui

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/titlebar"
	"github.com/xmhrnx/middlelan/internal/geo"
	"github.com/xmhrnx/middlelan/internal/inventory"
	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/internal/nettools"
	"github.com/xmhrnx/middlelan/internal/settings"
)

// Services are the app's long-lived parts the UI reads and drives.
type Services struct {
	App       *giode.App
	Collector *monitor.Collector
	Resolver  *geo.Resolver
	Settings  *settings.File
	Inventory *inventory.Inventory
	Scanner   *inventory.Scanner
}

// Shell is the root of the UI: navigation around the current page.
type Shell struct {
	svc     Services
	current screen
	toasts  *giode.ToasterElement

	sidebar   *sidebar
	bottomNav *bottomNav
	topBar    *topBar
	windowBar *titlebar.TitleBar

	dashboard   *dashboardScreen
	apps        *appsScreen
	detail      *appDetailScreen
	connections *connectionsScreen
	worldMap    *mapScreen
	devices     *devicesScreen
	tools       *toolsScreen
	settings    *settingsScreen

	// OnSettingsChanged lets main apply settings it owns (tray, keep-alive).
	OnSettingsChanged func(settings.Settings)

	redrawPending atomic.Bool
	public        atomic.Pointer[nettools.PublicInfo]
	netInfo       atomic.Pointer[nettools.NetInfo]
}

// NewShell builds the UI. Call it once, before App.Run.
func NewShell(svc Services) *Shell {
	sh := &Shell{svc: svc, toasts: giode.Toaster().Duration(5 * time.Second).Max(3)}
	applyTheme(svc.Settings.Get().Theme)
	sh.sidebar = newSidebar(sh)
	sh.bottomNav = newBottomNav(sh)
	sh.topBar = newTopBar(sh)
	sh.windowBar = newWindowBar(sh)
	sh.dashboard = newDashboardScreen(sh)
	sh.apps = newAppsScreen(sh)
	sh.detail = newAppDetailScreen(sh)
	sh.connections = newConnectionsScreen(sh)
	sh.worldMap = newMapScreen(sh)
	sh.devices = newDevicesScreen(sh)
	sh.tools = newToolsScreen(sh)
	sh.settings = newSettingsScreen(sh)
	return sh
}

// Redraw asks for a frame from any goroutine, coalescing bursts (the
// resolver can report many lookups at once).
func (sh *Shell) Redraw() {
	if sh.redrawPending.Swap(true) {
		return
	}
	sh.svc.App.Post(func() { sh.redrawPending.Store(false) })
}

// Toast shows a message from any goroutine.
func (sh *Shell) Toast(kind giode.ToastKind, msg string) {
	sh.toasts.Show(kind, msg)
}

// RefreshNetwork reloads the local and public network details in the
// background (at startup, and when the user asks).
func (sh *Shell) RefreshNetwork(ctx context.Context) {
	go func() {
		if ni, err := nettools.LocalNetInfo(); err == nil {
			sh.netInfo.Store(&ni)
			sh.Redraw()
		}
		if !sh.svc.Settings.Get().OnlineLookups {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if pi, err := nettools.PublicIPInfo(ctx); err == nil {
			sh.public.Store(&pi)
			sh.Redraw()
		}
	}()
}

// home is where this computer is, from its public address.
func (sh *Shell) home() (nettools.PublicInfo, bool) {
	p := sh.public.Load()
	if p == nil || (p.Lat == 0 && p.Lon == 0) {
		return nettools.PublicInfo{}, false
	}
	return *p, true
}

func (sh *Shell) navigate(s screen) {
	sh.current = s
}

// openApp shows the detail page of one app.
func (sh *Shell) openApp(name string) {
	sh.detail.show(name)
	sh.current = screenAppDetail
}

func (sh *Shell) togglePause() {
	c := sh.svc.Collector
	c.SetPaused(!c.Paused())
}

// page returns the current page's content.
func (sh *Shell) page() giode.Element {
	switch sh.current {
	case screenApps:
		return sh.apps.view()
	case screenAppDetail:
		giode.HandleBack(func() { sh.current = screenApps })
		return sh.detail.view()
	case screenConnections:
		return sh.connections.view()
	case screenMap:
		return sh.worldMap.view()
	case screenDevices:
		return sh.devices.view()
	case screenTools:
		return sh.tools.view()
	case screenSettings:
		return sh.settings.view()
	}
	return sh.dashboard.view()
}

// View is the whole window, every frame.
func (sh *Shell) View() giode.Element {
	body := giode.Responsive(
		giode.Breakpoint{MaxWidth: 760, Name: "compact", View: sh.compactView},
		giode.Breakpoint{View: sh.wideView},
	)
	if sh.windowBar != nil {
		body = giode.Stack(giode.Styles{Width: -1, Height: -1}, sh.windowBar, giode.Stack(giode.Styles{FlexGrow: 1, Width: -1}, body))
	}
	return giode.Box(giode.Styles{Width: -1, Height: -1, Background: "theme.background"}, body, sh.toasts)
}

func (sh *Shell) wideView() giode.Element {
	return giode.Row(giode.Styles{Width: -1, Height: -1, Wrap: giode.WrapNoWrap},
		sh.sidebar.view(),
		giode.Box(giode.Styles{Width: 1, Height: -1, Background: "theme.border"}),
		giode.Stack(giode.Styles{FlexGrow: 1, Height: -1, Width: -1}, sh.page()),
	)
}

func (sh *Shell) compactView() giode.Element {
	return giode.Stack(giode.Styles{Width: -1, Height: -1},
		sh.topBar.view(),
		giode.Stack(giode.Styles{FlexGrow: 1, Width: -1}, sh.page()),
		sh.bottomNav.view(),
	)
}

// pageBody wraps a page's content in the standard padding.
func pageBody(kids ...giode.Element) giode.Element {
	pad := giode.UniformInset(28)
	if giode.Compact() {
		pad = giode.UniformInset(14)
	}
	return giode.Stack(giode.Styles{Gap: 18, Padding: pad, Width: -1, MaxWidth: 1280}, compactKids(kids)...)
}

// compactKids drops nil elements, so views can return nil for "nothing".
func compactKids(kids []giode.Element) []giode.Element {
	out := kids[:0:0]
	for _, k := range kids {
		if k != nil {
			out = append(out, k)
		}
	}
	return out
}

// FormatRate formats bytes per second as a bit rate, for the tray.
func FormatRate(bytesPerSec float64) string { return formatRate(bytesPerSec) }
