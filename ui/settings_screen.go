package ui

import (
	"io"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/dropdown"
	"github.com/xmhrnx/giode/components/scroll"
	"github.com/xmhrnx/giode/components/segmented"
	"github.com/xmhrnx/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/export"
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
	exports   []*giode.ButtonElement
	checkNow  *giode.ButtonElement
	updates   *toggle.Switch
	language  *dropdown.Select
}

var scanIntervals = []int{5, 15, 30, 60}

func newSettingsScreen(sh *Shell) *settingsScreen {
	s := &settingsScreen{shell: sh}
	cur := sh.svc.Settings.Get()
	update := func(fn func(*settings.Settings)) {
		if err := sh.svc.Settings.Update(fn); err != nil {
			sh.Toast(giode.ToastError, tr("Couldn't save settings: {error}", "error", err.Error()))
		}
		sh.ApplySettings()
	}
	s.theme = segmentedT("Dark", "Light").OnChange(func(i int) {
		update(func(st *settings.Settings) { st.Theme = map[int]string{0: "dark", 1: "light"}[i] })
	})
	if cur.Theme == "light" {
		s.theme.SetSelected(1)
	}
	s.language = languagePicker(cur.Language, func(code string) {
		update(func(st *settings.Settings) { st.Language = code })
		applyLanguage(code)
	})
	sw := func(label string, on bool, set func(*settings.Settings, bool)) *toggle.Switch {
		t := switchT(label, giode.Styles{Color: "theme.text", FontSize: sizeBody, Width: -1},
			func(v bool) { update(func(st *settings.Settings) { set(st, v) }) })
		t.SetChecked(on)
		return t
	}
	s.online = sw(msg("Look up where remote servers are"), cur.OnlineLookups, func(st *settings.Settings, v bool) { st.OnlineLookups = v })
	s.newApps = sw(msg("Notify when an app connects for the first time"), cur.NotifyNewApps, func(st *settings.Settings, v bool) { st.NotifyNewApps = v })
	s.newDevs = sw(msg("Notify when a new device joins the network"), cur.NotifyNewDevices, func(st *settings.Settings, v bool) { st.NotifyNewDevices = v })
	s.bgScan = sw(msg("Scan the network in the background"), cur.BackgroundScan, func(st *settings.Settings, v bool) { st.BackgroundScan = v })
	s.fast = sw(msg("Faster live updates (every second)"), cur.FastSampling, func(st *settings.Settings, v bool) { st.FastSampling = v })
	// The system holds the login registration (the user can change it
	// there too), so the switch reads it rather than a saved setting.
	s.login = switchT("Start in the tray when I log in", giode.Styles{Color: "theme.text", FontSize: sizeBody, Width: -1}, func(on bool) {
		if err := sh.svc.App.SetLaunchAtLogin(on); err != nil {
			sh.Toast(giode.ToastWarning, tr("Couldn't change the login item: {error}", "error", err.Error()))
			s.login.SetChecked(sh.svc.App.LaunchAtLogin())
		}
	})
	s.login.SetChecked(!isMobile && sh.svc.App.LaunchAtLogin())
	s.tray = sw(msg("Keep monitoring when the window is closed"), cur.CloseToTray, func(st *settings.Settings, v bool) { st.CloseToTray = v })
	s.keepAlive = sw(msg("Keep monitoring in the background"), cur.KeepAliveAndroid, func(st *settings.Settings, v bool) { st.KeepAliveAndroid = v })
	s.scanEvery = selectT(styleField.With(giode.Styles{Width: 160}), "Every 5 minutes", "Every 15 minutes", "Every 30 minutes", "Every hour").OnChange(func(i int) {
		update(func(st *settings.Settings) { st.ScanEveryMinutes = scanIntervals[i] })
	})
	for i, m := range scanIntervals {
		if m == cur.ScanEveryMinutes {
			s.scanEvery.SetSelected(i)
		}
	}
	s.reset = buttonT("Clear usage history", styleOutline.With(giode.Styles{Color: "theme.danger", BorderColor: colDangerSoft})).Variant(giode.VariantOutlined).OnClick(func() {
		if !s.confirm {
			s.confirm = true
			s.reset.Label(tr("Click again to clear"))
			return
		}
		sh.svc.Collector.Store().Reset()
		s.confirm = false
		s.reset.Label(tr("Clear usage history"))
		sh.Toast(giode.ToastSuccess, tr("Usage history cleared"))
	})
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})
	exp := func(label, what, ext string, write func(export.Data, io.Writer) error) *giode.ButtonElement {
		return buttonT(label, small).Variant(giode.VariantOutlined).Icon(giode.Icon("file-down", giode.Styles{Width: 14})).
			OnClick(func() { sh.saveExport(exportName(what, ext), write) })
	}
	s.exports = []*giode.ButtonElement{
		exp(msg("Usage (CSV)"), "usage", "csv", export.Data.UsageCSV),
		exp(msg("Destinations (CSV)"), "destinations", "csv", export.Data.DestinationsCSV),
		exp(msg("Everything (JSON)"), "all", "json", export.Data.JSON),
	}
	s.updates = sw(msg("Check for updates once a day"), cur.CheckUpdates, func(st *settings.Settings, v bool) { st.CheckUpdates = v })
	s.checkNow = buttonT("Check now", small).Variant(giode.VariantOutlined).OnClick(func() { sh.checkUpdates(true) })
	s.scroll = giode.Scroll(s.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return s
}

func (s *settingsScreen) view() giode.Element { return s.scroll }

func (s *settingsScreen) content() giode.Element {
	section := func(title string, rows ...giode.Element) giode.Element {
		return card(title, nil, giode.Stack(giode.Styles{Gap: 14, Width: -1}, rows...))
	}
	note := func(text string) giode.Element { return giode.Text(text, styleMuted) }
	monitoring := []giode.Element{s.fast, note(tr("Sampling slows to every 6 seconds while the window is hidden, to save battery."))}
	if isMobile && !isIOS { // Android's foreground service
		monitoring = append(monitoring, s.keepAlive, note(tr("Shows an ongoing notification while monitoring in the background.")))
	} else {
		monitoring = append(monitoring, s.tray, s.login)
	}
	return pageBody(
		pageHeader(tr("Settings"), tr("Make MiddleMan yours")),
		section(tr("Appearance"), giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Text(tr("Theme"), giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}), s.theme),
			giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
				giode.Text(tr("Language"), giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}), s.language)),
		section(tr("Monitoring"), monitoring...),
		section(tr("Privacy"), s.online,
			note(tr("Sends remote IP addresses to geojs.io to find their country, city and owner. Turn off to keep everything on this device; host names still resolve through your DNS."))),
		section(tr("Alerts"), s.alertRows()...),
		section(tr("Data"), note(tr("Usage is kept for 90 days on this device only. Nothing is uploaded.")),
			giode.Row(giode.Styles{Gap: 8}, s.exports[0], s.exports[1], s.exports[2]),
			giode.Row(giode.Styles{}, s.reset)),
		s.about(),
	)
}

func (s *settingsScreen) about() giode.Element {
	version := giode.AppVersion
	if version == "" {
		version = tr("development build")
	}
	u := s.shell.update
	status := tr("Never checked")
	switch {
	case u.checking:
		status = tr("Checking…")
	case u.release != nil:
		status = tr("Version {version} is available", "version", u.release.Version)
	case u.err != "":
		status = tr("Last check failed: {error}", "error", u.err)
	case !u.checked.IsZero():
		status = tr("Up to date · checked {ago}", "ago", formatAgo(u.checked, time.Now()))
	}
	actions := []giode.Element{s.checkNow}
	if u.release != nil {
		s.shell.installBtn.Label(tr(map[bool]string{true: msg("Install {version}"), false: msg("Download {version}")}[u.release.Installable], "version", u.release.Version))
		actions = append(actions, s.shell.installBtn)
	}
	kids := []giode.Element{
		giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter},
			logoMark(48),
			giode.Stack(giode.Styles{Gap: 3},
				giode.Text("MiddleMan", styleHeading),
				giode.Text(tr("Version {version}", "version", version), styleMuted),
				giode.Text(tr("See every connection. Know every device."), styleMuted),
			)),
	}
	if !isMobile {
		kids = append(kids, giode.Divider(giode.Styles{Color: "theme.border"}), s.updates,
			giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Width: -1},
				giode.Text(status, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FlexGrow: 1}),
				giode.Row(giode.Styles{Gap: 8}, actions...)))
		if u.progress > 0 && u.progress < 1 {
			kids = append(kids, giode.Progress(u.progress, giode.Styles{Color: "theme.primary", Background: colGrid, Height: 4}))
		}
	}
	return cardStyled(giode.Styles{}, kids...)
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
	if !isMobile {
		sh.svc.App.SetCloseToTray(st.CloseToTray && giode.TraySupported())
	}
	if sh.OnSettingsChanged != nil {
		sh.OnSettingsChanged(st)
	}
}

// alertRows are the Alerts section's settings; iOS shows no other apps,
// so it has no new-app alerts.
func (s *settingsScreen) alertRows() []giode.Element {
	rows := []giode.Element{s.newApps, s.newDevs, s.bgScan}
	if isIOS {
		rows = rows[1:]
	}
	return append(rows, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
		giode.Text(tr("Background scan"), giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}), s.scanEvery))
}
