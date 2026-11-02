// Command middlelan is MiddleLAN, a network monitor: it shows which apps
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
	"slices"
	"strings"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"github.com/mehran9675/giode"

	"github.com/xmhrnx/middlelan/assets"
	"github.com/xmhrnx/middlelan/internal/autostart"
	"github.com/xmhrnx/middlelan/internal/geo"
	"github.com/xmhrnx/middlelan/internal/inventory"
	"github.com/xmhrnx/middlelan/internal/monitor"
	"github.com/xmhrnx/middlelan/internal/settings"
	"github.com/xmhrnx/middlelan/ui"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
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
	startHidden := slices.Contains(os.Args[1:], autostart.HiddenFlag)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var app *giode.App
	var shell *ui.Shell
	var collector *monitor.Collector
	resolver := geo.New(dir, st.OnlineLookups)
	collector = monitor.NewCollector(monitor.NewSource(), monitor.Options{
		Interval: 2 * time.Second, HiddenInterval: 6 * time.Second, DataDir: dir,
		OnUpdate: func() {
			if shell != nil {
				shell.Redraw()
				updateTray(app, collector)
			}
		},
		OnNewApp: func(name string) {
			if app != nil && prefs.Get().NotifyNewApps {
				notify(app, "New app on the network", name+" connected to the internet for the first time.")
			}
		},
	})
	inv := inventory.Open(dir)
	scanner := inventory.NewScanner(inv)

	app = giode.New(giode.Config{
		Title:     "MiddleLAN",
		Size:      image.Pt(1240, 800),
		MinSize:   image.Pt(380, 560),
		Resizable: true,
		Center:    true,
		// A custom title bar (ui/window_bar.go) replaces the OS frame on
		// desktops; phones have none.
		Frameless:  !isAndroid,
		Background: "theme.background",
		Icon:       assets.Icon,
		Minimized:  startHidden,
		// Closing hides to the tray so monitoring continues; Quit ends it.
		CloseToTray:    st.CloseToTray && giode.TraySupported(),
		MinimizeToTray: false,
		SingleInstanceLock: &giode.SingleInstanceLock{
			UniqueID:               "com.middlelan.app",
			OnSecondInstanceLaunch: func() { app.ShowWindow() },
		},
		OnReady: func() {
			if startHidden && giode.TraySupported() {
				app.HideWindow()
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

	shell = ui.NewShell(ui.Services{
		App: app, Collector: collector, Resolver: resolver, Settings: prefs, Inventory: inv, Scanner: scanner,
	})
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
		notify(app, "New device on your network", strings.Join(names, ", "))
	}

	setupTray(app, collector, shell)
	go collector.Run(ctx)
	go resolver.Run(ctx)
	go scanner.RunPeriodic(ctx,
		func() bool { return prefs.Get().BackgroundScan },
		func() time.Duration { return time.Duration(prefs.Get().ScanEveryMinutes) * time.Minute })
	shell.RefreshNetwork(ctx)

	return app.Run(shell.View)
}

// registerFonts loads Inter for the interface and the Go fonts for code
// and fallback glyphs.
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
	giode.RegisterFonts(append(faces, gofont.Collection()...))
	return nil
}

// dataDir is where MiddleLAN keeps its settings and history.
func dataDir() (string, error) {
	base, err := gioapp.DataDir()
	if err != nil {
		return "", fmt.Errorf("data folder: %w", err)
	}
	dir := filepath.Join(base, "MiddleLAN")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("data folder: %w", err)
	}
	return dir, nil
}

func notify(app *giode.App, title, body string) {
	if err := app.Notify(title, body); err != nil {
		log.Printf("notification: %v", err)
	}
}
