package ui

import (
	"context"
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dialog"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/mehran9675/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/alerts"
	"github.com/xmhrnx/middlelan/internal/firewall"
	"github.com/xmhrnx/middlelan/internal/settings"
)

// securityScreen gathers the protective features: threat alerts, the
// firewall (off by default) and how domain names are captured.
type securityScreen struct {
	shell       *Shell
	scroll      *scroll.Scroll
	threats     *toggle.Switch
	updateLists *giode.ButtonElement
	fwSwitch    *toggle.Switch
	ask         *toggle.Switch
	confirmOpen bool
	confirm     *dialog.Dialog
	enable      *giode.ButtonElement
	cancel      *giode.ButtonElement
	busy        bool
	vpn         *vpnControl
}

func newSecurityScreen(sh *Shell) *securityScreen {
	s := &securityScreen{shell: sh, vpn: newVPNControl(sh)}
	cur := sh.svc.Settings.Get()
	sw := func(label string, on bool, onChange func(bool)) *toggle.Switch {
		t := switchT(label, giode.Styles{Color: "theme.text", FontSize: sizeBody, Width: -1}, onChange)
		t.SetChecked(on)
		return t
	}
	s.threats = sw(msg("Warn about known malicious destinations"), cur.ThreatAlerts,
		func(on bool) { sh.updateSettings(func(st *settings.Settings) { st.ThreatAlerts = on }) })
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})
	s.updateLists = buttonT("Update lists now", small).Variant(giode.VariantOutlined).OnClick(func() {
		go func() {
			ctx, cancel := context.WithTimeout(sh.ctx, 3*time.Minute)
			defer cancel()
			if err := sh.svc.Threats.Update(ctx); err != nil {
				sh.Toast(giode.ToastWarning, tr("Some lists couldn't be updated: {error}", "error", err.Error()))
			} else {
				sh.Toast(giode.ToastSuccess, tr("Threat lists are up to date"))
			}
			sh.Redraw()
		}()
	})
	s.fwSwitch = sw(msg("Block the programs and addresses in my rules"), sh.svc.Firewall != nil && sh.svc.Firewall.Enabled(), func(on bool) {
		if on {
			s.fwSwitch.SetChecked(false) // until confirmed
			s.confirmOpen = true
			return
		}
		s.setFirewall(false)
	})
	s.ask = sw(msg("Ask me when a program connects for the first time"), cur.FirewallAsk,
		func(on bool) { sh.updateSettings(func(st *settings.Settings) { st.FirewallAsk = on }) })
	s.enable = buttonT("Turn on", stylePrimary).OnClick(func() {
		s.confirmOpen = false
		s.setFirewall(true)
	})
	s.cancel = buttonT("Cancel", styleGhost).Variant(giode.VariantGhost).OnClick(func() { s.confirmOpen = false })
	s.confirm = giode.Dialog(&s.confirmOpen, s.confirmContent, giode.Styles{Background: "theme.surface", BorderRadius: 18, Width: 480})
	s.scroll = giode.Scroll(s.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return s
}

// setFirewall turns the firewall on or off in the background (the system
// calls can take a moment and may ask for administrator rights).
func (s *securityScreen) setFirewall(on bool) {
	fw := s.shell.svc.Firewall
	if fw == nil || s.busy {
		return
	}
	s.busy = true
	go func() {
		var err error
		if on {
			err = fw.Enable()
		} else {
			err = fw.Disable()
		}
		s.shell.svc.App.Post(func() {
			s.busy = false
			s.fwSwitch.SetChecked(fw.Enabled())
			switch {
			case err != nil && on:
				s.shell.Toast(giode.ToastError, tr("Couldn't turn the firewall on: {error}", "error", err.Error()))
			case err != nil:
				s.shell.Toast(giode.ToastError, tr("Couldn't remove every rule: {error}", "error", err.Error()))
			case on:
				s.shell.Toast(giode.ToastSuccess, tr("Firewall on. Only the programs and addresses you block are affected."))
			default:
				s.shell.Toast(giode.ToastSuccess, tr("Firewall off. Every rule MiddleLAN added was removed."))
			}
		})
	}()
}

func (s *securityScreen) confirmContent() giode.Element {
	caps := s.shell.svc.Firewall.Capabilities()
	points := []string{
		tr("Nothing is blocked until you block a program or an address yourself."),
		tr("Your system's own firewall settings and defaults are never changed."),
		tr("Turning it off removes every rule MiddleLAN added."),
	}
	if caps.NeedsAdmin {
		points = append(points, tr("Applying rules needs administrator rights."))
	}
	if caps.Notes != "" {
		points = append(points, caps.Notes)
	}
	kids := []giode.Element{
		giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter}, iconBadge("shield-check", "theme.primary", 40), giode.Text(tr("Turn on the firewall?"), styleHeading)),
	}
	for _, p := range points {
		kids = append(kids, giode.Row(giode.Styles{Gap: 8, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Text("•", styleMuted), giode.Text(p, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1})))
	}
	kids = append(kids, giode.Row(giode.Styles{Gap: 8, Justify: giode.JustifyEnd, Width: -1}, s.cancel, s.enable))
	return giode.Stack(giode.Styles{Gap: 12, Width: -1}, kids...)
}

func (s *securityScreen) view() giode.Element {
	return giode.Box(giode.Styles{Width: -1, Height: -1}, s.scroll, s.confirm)
}

func (s *securityScreen) content() giode.Element {
	sh := s.shell
	active := 0
	if sh.svc.Alerts != nil {
		active = sh.svc.Alerts.Active()
	}
	sub := tr("No threats found")
	if active > 0 {
		sub = trn("{count} alerts need your attention", active)
	}
	return pageBody(
		pageHeader(tr("Security"), sub),
		s.alertsCard(),
		s.firewallCard(),
		s.vpn.card(),
		s.threatCard(),
		s.namesCard(),
	)
}

func (s *securityScreen) alertsCard() giode.Element {
	sh := s.shell
	if sh.svc.Alerts == nil {
		return nil
	}
	list := sh.svc.Alerts.List()
	rows := []giode.Element{}
	now := time.Now()
	for i, a := range list {
		if i == 30 {
			break
		}
		rows = append(rows, sh.alertRow(a, now))
	}
	if len(rows) == 0 {
		rows = append(rows, emptyState("shield-check", tr("All clear"), tr("No app has contacted an address on the threat lists."), nil))
	}
	return card(tr("Threat alerts"), nil, giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
}

// alertRow is one alert with its actions, kept by key so its buttons
// keep their state.
func (sh *Shell) alertRow(a alerts.Alert, now time.Time) giode.Element {
	type rowState struct{ dismiss, block *giode.ButtonElement }
	key := a.Key()
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(10, 5), FontSize: sizeTiny})
	st := giode.Keyed("alert-row", key, func() *rowState {
		return &rowState{
			dismiss: giode.Button(tr("Dismiss"), small).Variant(giode.VariantOutlined).OnClick(func() { sh.svc.Alerts.Dismiss(key) }),
			block: giode.Button(tr("Block address"), small.With(giode.Styles{Color: "theme.danger", BorderColor: colDangerSoft})).Variant(giode.VariantOutlined).
				OnClick(func() { sh.blockRule(firewall.Rule{Dest: a.IP, Note: tr("Threat list: {list}", "list", a.List)}) }),
		}
	})
	st.dismiss.Label(tr("Dismiss"))
	st.block.Label(tr("Block address"))
	target := a.IP
	if a.Domain != "" {
		target = a.Domain + " (" + a.IP + ")"
	}
	tone := giode.Color("theme.danger")
	if a.Dismissed {
		tone = "theme.muted"
	}
	actions := []giode.Element{}
	if !a.Dismissed {
		actions = append(actions, st.dismiss)
	}
	if sh.svc.Firewall != nil && sh.svc.Firewall.Capabilities().PerDestination {
		actions = append(actions, st.block)
	}
	return giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(2, 8), Width: -1, Wrap: giode.WrapNoWrap},
		iconBadge("triangle-alert", tone, 34),
		giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1, Width: -1},
			giode.Text(a.App+arrowForward()+target, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(strings.Trim(a.Category+" · "+a.List+" · "+a.Detail, " ·"), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(tr("First {first} · last {last}", "first", formatAgo(a.FirstSeen, now), "last", formatAgo(a.LastSeen, now))+" · "+plural(a.Count, "time", "times"), styleMuted),
		),
		giode.Row(giode.Styles{Gap: 6, Wrap: giode.WrapNoWrap}, actions...),
	)
}

func (s *securityScreen) firewallCard() giode.Element {
	fw := s.shell.svc.Firewall
	if fw == nil {
		return nil
	}
	st := fw.Status()
	caps := fw.Capabilities()
	s.fwSwitch.SetChecked(st.Enabled)
	kids := []giode.Element{
		s.fwSwitch,
		giode.Text(tr("Off by default. MiddleLAN never changes your system's firewall defaults: it only adds the block rules you create here, and turning it off removes all of them.")+" "+caps.Backend+".", styleMuted),
	}
	if s.busy {
		kids = append(kids, giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter}, giode.Spinner(giode.Styles{Width: 14, Color: "theme.primary"}), giode.Text(tr("Applying…"), styleMuted)))
	}
	if st.SystemFirewallOff {
		kids = append(kids, noticeBanner("shield-alert", tr("Your system's firewall is turned off, so these rules have no effect. MiddleLAN won't turn it on for you."), "theme.warning", nil))
	}
	for _, e := range st.Errors {
		kids = append(kids, noticeBanner("info", e, "theme.warning", nil))
	}
	if st.Enabled {
		kids = append(kids, s.ask)
	}
	rules := fw.Rules()
	rows := []giode.Element{}
	for _, r := range rules {
		rows = append(rows, s.shell.ruleRow(r))
	}
	if len(rows) == 0 {
		rows = append(rows, giode.Text(tr("No rules yet. Block a program from its page, or an address from an alert."), styleMuted))
	}
	kids = append(kids, giode.Text(tr("Rules"), styleLabel), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
	return card(tr("Firewall"), nil, kids...)
}

// ruleRow shows a block rule with a pause switch and a remove button.
func (sh *Shell) ruleRow(r firewall.Rule) giode.Element {
	type rowState struct {
		on     *toggle.Switch
		remove *giode.ButtonElement
	}
	id := r.ID
	st := giode.Keyed("fw-rule", id, func() *rowState {
		rs := &rowState{
			on: giode.Input.Switch(""),
			remove: giode.Button(tr("Remove"), styleGhost.With(giode.Styles{Padding: giode.SymmetricInset(10, 5), FontSize: sizeTiny, Color: "theme.danger"})).Variant(giode.VariantGhost).
				OnClick(func() { sh.firewallChange(func(fw *firewall.Firewall) error { return fw.Remove(id) }) }),
		}
		rs.on.OnChange(func(on bool) {
			sh.firewallChange(func(fw *firewall.Firewall) error {
				for _, cur := range fw.Rules() {
					if cur.ID == id {
						cur.Enabled = on
						return fw.Update(cur)
					}
				}
				return nil
			})
		})
		return rs
	})
	st.on.SetChecked(r.Enabled)
	st.remove.Label(tr("Remove"))
	what := tr("Every program")
	if !r.AnyApp() {
		what = nonEmpty(r.App, r.Exe)
	}
	where := tr("all destinations")
	if r.Dest != "" {
		where = r.Dest
	}
	return giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Padding: giode.SymmetricInset(2, 6), Wrap: giode.WrapNoWrap},
		iconBadge("ban", "theme.danger", 30),
		giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1, Width: -1},
			giode.Text(what+arrowForward()+where, giode.Styles{Color: "theme.text", FontSize: sizeBody, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(nonEmpty(r.Note, tr("Added {date}", "date", r.Created.Local().Format("2006-01-02"))), styleMuted)),
		st.on, st.remove)
}

// firewallChange runs a firewall edit in the background and reports errors.
func (sh *Shell) firewallChange(edit func(*firewall.Firewall) error) {
	fw := sh.svc.Firewall
	if fw == nil {
		return
	}
	go func() {
		if err := edit(fw); err != nil {
			sh.Toast(giode.ToastError, tr("Firewall: {error}", "error", err.Error()))
		}
		sh.Redraw()
	}()
}

// blockRule adds a block rule; with the firewall off it's saved and takes
// effect when the firewall is turned on.
func (sh *Shell) blockRule(r firewall.Rule) {
	r.Enabled = true
	sh.firewallChange(func(fw *firewall.Firewall) error {
		if _, err := fw.Add(r); err != nil {
			return err
		}
		what := nonEmpty(r.App, r.Dest)
		if fw.Enabled() {
			sh.Toast(giode.ToastSuccess, tr("Blocked {target}", "target", what))
		} else {
			sh.Toast(giode.ToastInfo, tr("Rule saved for {target}. It applies once you turn the firewall on in Security.", "target", what))
		}
		return nil
	})
}

func (s *securityScreen) threatCard() giode.Element {
	db := s.shell.svc.Threats
	if db == nil {
		return nil
	}
	rows := []giode.Element{}
	for _, l := range db.Lists() {
		status := plural(l.Entries, "entry", "entries")
		if !l.Updated.IsZero() {
			status += " · " + tr("updated {ago}", "ago", formatAgo(l.Updated, time.Now()))
		}
		if l.Err != "" {
			status += " · " + tr("last update failed")
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: 10, Width: -1, Padding: giode.SymmetricInset(0, 4)},
			giode.Text(l.Name, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1}),
			giode.Text(l.Category, styleMuted), giode.Text(status, styleMuted)))
	}
	return card(tr("Threat lists"), s.updateLists, s.threats,
		giode.Text(tr("Free public blocklists (abuse.ch, Spamhaus, FireHOL), refreshed daily and checked on this device. Your traffic isn't sent anywhere."), styleMuted),
		giode.Stack(giode.Styles{Width: -1}, rows...))
}

func (s *securityScreen) namesCard() giode.Element {
	if s.shell.svc.Names == nil {
		return nil
	}
	var action giode.Element
	if !giode.IsElevated() && canElevate() && strings.HasPrefix(s.shell.svc.Names.Method(), "Unavailable") {
		action = s.shell.dashboard.notices.elevate
	}
	return card(tr("Domain names"), action,
		giode.Text(tr("How MiddleLAN learns which sites apps look up: {method}.", "method", s.shell.svc.Names.Method()), giode.Styles{Color: "theme.text", FontSize: sizeSmall}),
		giode.Text(tr("Names from encrypted DNS (DNS over HTTPS, used by some browsers) can't be seen; reverse DNS fills in where possible."), styleMuted))
}
