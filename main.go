// Command middlelan is MiddleMan, a network monitor: it shows which apps
// use the network and where they connect, finds the devices on the local
// network, and carries a full set of network tools.
package main

import (
	"context"
	"fmt"
	"image"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/middlelan/assets"
	"github.com/xmhrnx/middlelan/internal/alerts"
	"github.com/xmhrnx/middlelan/internal/dnslog"
	"github.com/xmhrnx/middlelan/internal/firewall"
	"github.com/xmhrnx/middlelan/internal/geo"
	"github.com/xmhrnx/middlelan/internal/inventory"
	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/internal/router"
	"github.com/xmhrnx/middlelan/internal/settings"
	"github.com/xmhrnx/middlelan/internal/speedlog"
	"github.com/xmhrnx/middlelan/internal/threat"
	"github.com/xmhrnx/middlelan/internal/updates"
	"github.com/xmhrnx/middlelan/internal/vpn"
	"github.com/xmhrnx/middlelan/locales"
	"github.com/xmhrnx/middlelan/ui"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Names the login entry and other registrations; the giode CLI sets it
	// from giode.toml, a plain go build doesn't.
	if giode.AppID == "" {
		giode.AppID = "com.middlelan.app"
	}
	if err := registerFonts(); err != nil {
		return err
	}
	if err := giode.RegisterIconsFS(assets.Icons, "icons"); err != nil {
		return fmt.Errorf("icons: %w", err)
	}
	dir, err := dataDir()
	if err != nil {
		return err
	}
	prefs, err := settings.Open(dir)
	if err != nil {
		log.Printf("settings: %v (using defaults)", err)
	}
	st := prefs.Get()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var app *giode.App
	var shell *ui.Shell
	var collector *monitor.Collector
	var tray *trayPanel
	var watcher *alerts.Watcher
	var fw *firewall.Firewall
	resolver := geo.New(dir, st.OnlineLookups)
	names := dnslog.New()
	resolver.SetDomains(names.Lookup)
	collector = monitor.NewCollector(monitor.NewSource(), monitor.Options{
		Interval: 2 * time.Second, HiddenInterval: 6 * time.Second, DataDir: dir,
		OnUpdate: func() {
			if shell != nil {
				snap := collector.Snapshot()
				shell.Redraw()
				tray.refresh(false)
				shell.OnSample(snap)
				if watcher != nil && prefs.Get().ThreatAlerts {
					for _, a := range watcher.Check(snap.Conns, snap.Time) {
						notify(app, giode.Intl("Suspicious connection"), giode.Intl("{app} contacted {ip}, listed for {category} ({list}).",
							"app", a.App, "ip", a.IP, "category", a.Category, "list", a.List))
					}
				}
			}
		},
		OnNewApp: func(name string) {
			p := prefs.Get()
			if shell != nil && fw != nil && fw.Enabled() && p.FirewallAsk {
				shell.AskAboutApp(name)
			} else if app != nil && p.NotifyNewApps {
				notify(app, giode.Intl("New app on the network"), giode.Intl("{app} connected to the internet for the first time.", "app", name))
			}
		},
	})
	inv := inventory.Open(dir)
	scanner := inventory.NewScanner(inv)
	gateway := &router.Watcher{}
	threats := threat.Open(dir)
	watcher = alerts.New(threats, names.Lookup, dir)
	fw, err = firewall.Open(dir, firewall.DefaultBackend())
	if err != nil {
		// The firewall is optional: without its saved state it starts
		// off, which is the default anyway.
		log.Printf("firewall: %v", err)
	}
	if fw != nil {
		// Re-resolve host rules, or, if the firewall is off but the last
		// run crashed with rules installed, remove them.
		go func() {
			if err := fw.Refresh(); err != nil {
				log.Printf("firewall: %v", err)
			}
		}()
	}

	app = giode.New(giode.Config{
		Title:     "MiddleMan",
		Size:      image.Pt(1240, 800),
		MinSize:   image.Pt(380, 560),
		Resizable: true,
		Center:    true,
		// The interface language: the saved choice, else the system's.
		Locale: st.Language,
		Intl:   &giode.IntlOptions{FS: locales.FS, Dir: ".", Fallback: "en"},
		// A custom title bar (ui/window_bar.go) replaces the OS frame on
		// desktops; phones have none.
		Frameless:  !isAndroid,
		Background: "theme.background",
		Icon:       assets.Icon,
		MCP:        debugMCP,
		// Closing hides to the tray so monitoring continues; Quit ends it.
		CloseToTray: st.CloseToTray && giode.TraySupported(),
		// Started at login (Settings → Start in the tray), the window
		// stays hidden and only the tray icon shows.
		StartInTray:        giode.StartInTrayAtLogin,
		SingleInstanceLock: singleInstance(func() { app.ShowWindow() }),
		OnReady: func() {
			if giode.LaunchedAtLogin() && giode.TraySupported() {
				collector.SetVisible(false)
			}
			applyKeepAlive(app, prefs.Get())
		},
		OnBackground:  func() { collector.SetVisible(false) },
		OnForeground:  func() { collector.SetVisible(true) },
		OnBeforeClose: func() { collector.SetVisible(false) },
		OnWindowStateChange: func(s giode.WindowState) {
			collector.SetVisible(s != giode.WindowMinimized)
		},
		OnShutdown: cancel,
	})

	shell = ui.NewShell(ctx, ui.Services{
		App: app, Collector: collector, Resolver: resolver, Settings: prefs, Inventory: inv, Scanner: scanner,
		Router: gateway, Speed: speedlog.Open(dir), Updates: updates.New(giode.AppVersion),
		Firewall: fw, Alerts: watcher, Threats: threats, Names: names,
	})
	gateway.OnChange = shell.Redraw
	shell.OnSettingsChanged = func(s settings.Settings) { applyKeepAlive(app, s) }
	resolver.OnChange(shell.Redraw)
	scanner.OnChange = shell.Redraw
	scanner.OnNew = func(devs []inventory.Device) {
		if !prefs.Get().NotifyNewDevices {
			return
		}
		names := make([]string, 0, len(devs))
		for _, d := range devs {
			names = append(names, d.Name()+" ("+d.IP+")")
		}
		notify(app, giode.Intl("New device on your network"), strings.Join(names, ", "))
	}

	tray = setupTray(app, collector, shell)
	go collector.Run(ctx)
	// The router is polled every 10 s while the window shows, every
	// minute in the background.
	go gateway.Run(ctx, func() time.Duration {
		if app.WindowVisible() {
			return 10 * time.Second
		}
		return time.Minute
	})
	shell.Start(ctx)
	// The Android VPN: new flows obey the firewall rules, and the DNS
	// answers it relays give domain names. (A no-op elsewhere.)
	vpn.SetHooks(vpn.Hooks{
		Block: func(app, pkg, ip string, port int) bool { return fw != nil && fw.Blocks(app, pkg, ip) },
		DNS: func(resp []byte) {
			for _, a := range dnslog.ParseResponse(resp, time.Now()) {
				names.Add(a)
			}
		},
	})
	go threats.Run(ctx, 24*time.Hour, func() bool { return prefs.Get().ThreatAlerts })
	go resolver.Run(ctx)
	go func() {
		if err := names.Run(ctx); err != nil {
			log.Printf("domain names: %v", err)
		}
	}()
	go scanner.RunPeriodic(ctx,
		func() bool { return prefs.Get().BackgroundScan },
		func() time.Duration { return time.Duration(prefs.Get().ScanEveryMinutes) * time.Minute })
	shell.RefreshNetwork(ctx)

	return app.Run(func() giode.Element {
		allowViewReattach()
		return shell.View()
	})
}

// registerFonts loads Inter for the interface, Vazirmatn for Persian
// (Arabic script, which Inter lacks) and the Go fonts for code and
// fallback glyphs.
func registerFonts() error {
	weights := map[string]font.Weight{"Regular": font.Normal, "Medium": font.Medium, "SemiBold": font.SemiBold, "Bold": font.Bold}
	var faces []font.FontFace
	for name, w := range weights {
		b, err := fs.ReadFile(assets.Fonts, "fonts/Inter-"+name+".otf")
		if err != nil {
			return fmt.Errorf("font %s: %w", name, err)
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return fmt.Errorf("font %s: %w", name, err)
		}
		faces = append(faces, font.FontFace{Font: font.Font{Typeface: "Inter", Weight: w}, Face: face})
	}
	for name, w := range map[string]font.Weight{"Regular": font.Normal, "Bold": font.Bold} {
		b, err := fs.ReadFile(assets.Fonts, "fonts/Vazirmatn-"+name+".ttf")
		if err != nil {
			return fmt.Errorf("font Vazirmatn %s: %w", name, err)
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return fmt.Errorf("font Vazirmatn %s: %w", name, err)
		}
		faces = append(faces, font.FontFace{Font: font.Font{Typeface: "Vazirmatn", Weight: w}, Face: face})
	}
	giode.RegisterFonts(append(faces, gofont.Collection()...))
	return nil
}

// dataDir is where MiddleMan keeps its settings and history.
func dataDir() (string, error) {
	base, err := gioapp.DataDir()
	if err != nil {
		return "", fmt.Errorf("data folder: %w", err)
	}
	dir := filepath.Join(base, "MiddleMan")
	// The app was called MiddleLAN: move its data over on first start,
	// so history, devices and firewall rules carry over.
	old := filepath.Join(base, "MiddleLAN")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if _, err := os.Stat(old); err == nil {
			if err := os.Rename(old, dir); err != nil {
				log.Printf("data folder: couldn't move %s to %s: %v", old, dir, err)
			}
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("data folder: %w", err)
	}
	return dir, nil
}

// singleInstance keeps a second launch on desktops from starting another
// monitor; it brings the running window forward instead. Android already
// runs one instance per app, and giode's lock file lives in the temp
// folder, which Android apps can't write: there the lock would report
// "already running" and the app would exit before opening.
func singleInstance(show func()) *giode.SingleInstanceLock {
	if isAndroid {
		return nil
	}
	return &giode.SingleInstanceLock{UniqueID: "com.middlelan.app", OnSecondInstanceLaunch: show}
}

func notify(app *giode.App, title, body string) {
	if err := app.Notify(title, body); err != nil {
		log.Printf("notification: %v", err)
	}
}
