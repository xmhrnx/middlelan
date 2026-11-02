package ui

import (
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/mehran9675/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/autostart"
	"github.com/xmhrnx/middlelan/internal/settings"
)

// settingsScreen holds the preferences and the about box.
type settingsScreen struct {
	shell     *Shell
	scroll    *scroll.Scroll
	theme     *segmented.Control
	online    *toggle.Switch
	newApps   *toggle.Switch
	newDevs   *toggle.Switch
	bgScan    *toggle.Switch
	scanEvery *dropdown.Select
	fast      *toggle.Switch
	login     *toggle.Switch
	tray      *toggle.Switch
	keepAlive *toggle.Switch
	reset     *giode.ButtonElement
	confirm   bool
}

var scanIntervals = []int{5, 15, 30, 60}

func newSettingsScreen(sh *Shell) *settingsScreen {
	s := &settingsScreen{shell: sh}
	cur := sh.svc.Settings.Get()
	update := func(fn func(*settings.Settings)) {
		if err := sh.svc.Settings.Update(fn); err != nil {
			sh.Toast(giode.ToastError, "Couldn't save settings: "+err.Error())
		}
		sh.ApplySettings()
	}
	s.theme = giode.Input.Segmented("Dark", "Light").OnChange(func(i int) {
		update(func(st *settings.Settings) { st.Theme = map[int]string{0: "dark", 1: "light"}[i] })
	})
	if cur.Theme == "light" {
		s.theme.SetSelected(1)
	}
	sw := func(label string, on bool, set func(*settings.Settings, bool)) *toggle.Switch {
		t := giode.Input.Switch(label, giode.Styles{Color: "theme.text", FontSize: sizeBody, Width: -1})
		t.SetChecked(on)
		t.OnChange(func(v bool) { update(func(st *settings.Settings) { set(st, v) }) })
		return t
	}
	s.online = sw("Look up where remote servers are", cur.OnlineLookups, func(st *settings.Settings, v bool) { st.OnlineLookups = v })
	s.newApps = sw("Notify when an app connects for the first time", cur.NotifyNewApps, func(st *settings.Settings, v bool) { st.NotifyNewApps = v })
	s.newDevs = sw("Notify when a new device joins the network", cur.NotifyNewDevices, func(st *settings.Settings, v bool) { st.NotifyNewDevices = v })
	s.bgScan = sw("Scan the network in the background", cur.BackgroundScan, func(st *settings.Settings, v bool) { st.BackgroundScan = v })
	s.fast = sw("Faster live updates (every second)", cur.FastSampling, func(st *settings.Settings, v bool) { st.FastSampling = v })
	s.login = sw("Start hidden when I log in", cur.StartAtLogin, func(st *settings.Settings, v bool) { st.StartAtLogin = v })
	s.tray = sw("Keep monitoring when the window is closed", cur.CloseToTray, func(st *settings.Settings, v bool) { st.CloseToTray = v })
	s.keepAlive = sw("Keep monitoring in the background", cur.KeepAliveAndroid, func(st *settings.Settings, v bool) { st.KeepAliveAndroid = v })
	s.scanEvery = giode.Input.Select(styleField.With(giode.Styles{Width: 160}), "Every 5 minutes", "Every 15 minutes", "Every 30 minutes", "Every hour").OnChange(func(i int) {
		update(func(st *settings.Settings) { st.ScanEveryMinutes = scanIntervals[i] })
	})
	for i, m := range scanIntervals {
		if m == cur.ScanEveryMinutes {
			s.scanEvery.SetSelected(i)
		}
	}
	s.reset = giode.Button("Clear usage history", styleOutline.With(giode.Styles{Color: "theme.danger", BorderColor: colDangerSoft})).Variant(giode.VariantOutlined).OnClick(func() {
		if !s.confirm {
			s.confirm = true
			s.reset.Label("Click again to clear")
			return
		}
		sh.svc.Collector.Store().Reset()
		s.confirm = false
		s.reset.Label("Clear usage history")
		sh.Toast(giode.ToastSuccess, "Usage history cleared")
	})
	s.scroll = giode.Scroll(s.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return s
}

func (s *settingsScreen) view() giode.Element { return s.scroll }

func (s *settingsScreen) content() giode.Element {
	section := func(title string, rows ...giode.Element) giode.Element {
		return card(title, nil, giode.Stack(giode.Styles{Gap: 14, Width: -1}, rows...))
	}
	note := func(text string) giode.Element { return giode.Text(text, styleMuted) }
	monitoring := []giode.Element{s.fast, note("Sampling slows to every 6 seconds while the window is hidden, to save battery.")}
	if isMobile {
		monitoring = append(monitoring, s.keepAlive, note("Shows an ongoing notification while monitoring in the background."))
	} else {
		monitoring = append(monitoring, s.tray)
		if autostart.Supported() {
			monitoring = append(monitoring, s.login)
		}
	}
	return pageBody(
		pageHeader("Settings", "Make MiddleLAN yours"),
		section("Appearance", giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Text("Theme", giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}), s.theme)),
		section("Monitoring", monitoring...),
		section("Privacy", s.online,
			note("Sends remote IP addresses to geojs.io to find their country, city and owner. Turn off to keep everything on this device; host names still resolve through your DNS.")),
		section("Alerts", s.newApps, s.newDevs, s.bgScan,
			giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
				giode.Text("Background scan", giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}), s.scanEvery)),
		section("Data", note("Usage is kept for 90 days on this device only. Nothing is uploaded."), giode.Row(giode.Styles{}, s.reset)),
		s.about(),
	)
}

func (s *settingsScreen) about() giode.Element {
	version := giode.AppVersion
	if version == "" {
		version = "development build"
	}
	return cardStyled(giode.Styles{},
		giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter},
			logoMark(48),
			giode.Stack(giode.Styles{Gap: 3},
				giode.Text("MiddleLAN", styleHeading),
				giode.Text("Version "+version, styleMuted),
				giode.Text("See every connection. Know every device.", styleMuted),
			)),
	)
}

// ApplySettings puts the current settings into effect.
func (sh *Shell) ApplySettings() {
	st := sh.svc.Settings.Get()
	applyTheme(st.Theme)
	sh.svc.Resolver.SetOnline(st.OnlineLookups)
	if st.FastSampling {
		sh.svc.Collector.SetInterval(time.Second)
	} else {
		sh.svc.Collector.SetInterval(0)
	}
	if autostart.Supported() {
		var err error
		if st.StartAtLogin {
			err = autostart.Enable()
		} else {
			err = autostart.Disable()
		}
		if err != nil {
			sh.Toast(giode.ToastWarning, "Couldn't change the login item: "+err.Error())
		}
	}
	if sh.OnSettingsChanged != nil {
		sh.OnSettingsChanged(st)
	}
}
