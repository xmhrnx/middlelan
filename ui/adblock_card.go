package ui

import (
	"context"
	"slices"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/adblock"
	"github.com/xmhrnx/middlelan/internal/settings"
	"github.com/xmhrnx/middlelan/internal/vpn"
)

// adBlockControl is the ad and tracker blocking section of the Security
// page: the on-device VPN answers queries for listed domains itself.
type adBlockControl struct {
	shell    *Shell
	vpn      *vpnControl
	sw       *toggle.Switch
	update   *giode.ButtonElement
	updating bool
}

func newAdBlockControl(sh *Shell, v *vpnControl) *adBlockControl {
	c := &adBlockControl{shell: sh, vpn: v}
	c.sw = switchT(msg("Block ads and trackers"), giode.Styles{Color: "theme.text", FontSize: sizeBody, Width: -1}, func(on bool) {
		sh.updateSettings(func(st *settings.Settings) { st.AdBlock = on })
		if on {
			if !vpn.Default.Running() {
				v.start()
			}
			if sh.svc.AdBlock.Domains() == 0 {
				c.refresh(false)
			}
		}
	})
	c.sw.SetChecked(sh.svc.Settings.Get().AdBlock)
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})
	c.update = buttonT("Update lists now", small).Variant(giode.VariantOutlined).OnClick(func() { c.refresh(true) })
	return c
}

// refresh downloads the lists in the background.
func (c *adBlockControl) refresh(say bool) {
	if c.updating {
		return
	}
	c.updating = true
	sh := c.shell
	go func() {
		ctx, cancel := context.WithTimeout(sh.ctx, 3*time.Minute)
		defer cancel()
		err := sh.svc.AdBlock.Update(ctx)
		sh.svc.App.Post(func() { c.updating = false })
		switch {
		case err != nil:
			sh.Toast(giode.ToastWarning, tr("Some lists couldn't be updated: {error}", "error", err.Error()))
		case say:
			sh.Toast(giode.ToastSuccess, tr("Ad blocking lists are up to date"))
		}
		sh.Redraw()
	}()
}

// allow stops blocking a domain (and its subdomains).
func (sh *Shell) allowDomain(name string) {
	sh.updateSettings(func(st *settings.Settings) {
		if !slices.Contains(st.AdBlockAllow, name) {
			st.AdBlockAllow = append(st.AdBlockAllow, name)
		}
	})
	sh.Toast(giode.ToastSuccess, tr("{domain} is no longer blocked", "domain", name))
}

// disallow blocks a domain again.
func (sh *Shell) disallowDomain(name string) {
	sh.updateSettings(func(st *settings.Settings) {
		st.AdBlockAllow = slices.DeleteFunc(st.AdBlockAllow, func(s string) bool { return s == name })
	})
}

// card is nil where there's no on-device VPN.
func (c *adBlockControl) card() giode.Element {
	if !vpn.Supported() || c.shell.svc.AdBlock == nil {
		return nil
	}
	sh := c.shell
	b := sh.svc.AdBlock
	prefs := sh.svc.Settings.Get()
	c.sw.SetChecked(prefs.AdBlock)
	kids := []giode.Element{
		c.sw,
		giode.Text(tr("Answers requests for ad, tracking and analytics domains on the phone itself, so they never load, in every app and browser. Uses open source blocklists: the AdGuard DNS filter and StevenBlack's hosts list, updated daily."), styleMuted),
	}
	if !prefs.AdBlock {
		return card(tr("Ad and tracker blocking"), nil, kids...)
	}
	if !vpn.Default.Running() && !c.vpn.starting {
		kids = append(kids, noticeBanner("shield-alert", tr("Blocking works while the on-device VPN is on."), "theme.warning",
			buttonT("Turn on", stylePrimary.With(giode.Styles{Padding: giode.SymmetricInset(12, 6), FontSize: sizeSmall})).OnClick(c.vpn.start)))
	}
	st := b.Stats()
	now := time.Now()
	var updated time.Time
	listErr := ""
	for _, l := range b.Lists() {
		if l.Updated.After(updated) {
			updated = l.Updated
		}
		if l.Err != "" && listErr == "" {
			listErr = l.Name + ": " + l.Err
		}
	}
	lists := tr("Downloading the lists…")
	if !c.updating {
		lists = tr("Lists not downloaded yet")
		if !updated.IsZero() {
			lists = tr("{count} domains · updated {ago}", "count", formatCount(b.Domains()), "ago", formatAgo(updated, now))
		}
	}
	kids = append(kids,
		responsiveGrid(140, 3, 12,
			miniStat(tr("Blocked today"), formatCount(st.Today)),
			miniStat(tr("Since MiddleMan started"), formatCount(st.Total)),
			miniStat(tr("Apps affected today"), itoa(len(st.TopApps))),
		),
		giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Width: -1},
			giode.Text(lists, styleMuted.With(giode.Styles{FlexGrow: 1})), c.update),
	)
	if listErr != "" {
		kids = append(kids, giode.Text(listErr, giode.Styles{Color: "theme.warning", FontSize: sizeTiny}))
	}
	if len(st.TopDomains) > 0 {
		rows := []giode.Element{}
		for i, d := range st.TopDomains {
			if i == 10 {
				break
			}
			rows = append(rows, sh.blockedDomainRow(d, slices.Contains(prefs.AdBlockAllow, d.Name)))
		}
		kids = append(kids, giode.Text(tr("Most blocked today"), styleLabel), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
	}
	if len(prefs.AdBlockAllow) > 0 {
		rows := []giode.Element{}
		for _, d := range prefs.AdBlockAllow {
			rows = append(rows, sh.allowedDomainRow(d))
		}
		kids = append(kids, giode.Text(tr("Never blocked"), styleLabel), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
	}
	return card(tr("Ad and tracker blocking"), nil, kids...)
}

// miniStat is a label over a number.
func miniStat(label, value string) giode.Element {
	return giode.Stack(giode.Styles{Gap: 2, Padding: giode.UniformInset(12), Background: colSurface2, BorderRadius: 12},
		giode.Text(value, giode.Styles{Color: "theme.text", FontSize: 20, FontWeight: giode.FontWeightBold}),
		giode.Text(label, styleMuted),
	)
}

// blockedDomainRow is one blocked domain with how often, and Allow.
func (sh *Shell) blockedDomainRow(d adblock.Count, allowed bool) giode.Element {
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(10, 5), FontSize: sizeTiny})
	allow := giode.Keyed("adblock-allow", d.Name, func() *giode.ButtonElement {
		name := d.Name
		return giode.Button(tr("Allow"), small).Variant(giode.VariantOutlined).OnClick(func() { sh.allowDomain(name) })
	})
	allow.Label(tr("Allow"))
	var action giode.Element = allow
	if allowed {
		action = giode.Text(tr("Allowed"), giode.Styles{Color: "theme.muted", FontSize: sizeTiny})
	}
	row := giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Padding: giode.SymmetricInset(2, 8), Width: -1, Wrap: giode.WrapNoWrap},
		giode.Text(d.Name, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1, Width: -1, TextOverflow: giode.TextOverflowEllipsis}),
		giode.Text(itoa(d.Count), giode.Styles{Color: "theme.muted", FontSize: sizeSmall}),
		action,
	)
	return withMenu(row, sh.blockedDomainMenu(d.Name))
}

// allowedDomainRow is one domain the user allowed, with Block again.
func (sh *Shell) allowedDomainRow(name string) giode.Element {
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(10, 5), FontSize: sizeTiny})
	again := giode.Keyed("adblock-disallow", name, func() *giode.ButtonElement {
		return giode.Button(tr("Block again"), small).Variant(giode.VariantOutlined).OnClick(func() { sh.disallowDomain(name) })
	})
	again.Label(tr("Block again"))
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 10, Padding: giode.SymmetricInset(2, 8), Width: -1, Wrap: giode.WrapNoWrap},
		giode.Text(name, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1, Width: -1, TextOverflow: giode.TextOverflowEllipsis}),
		again,
	)
}
