package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/menu"
	"github.com/xmhrnx/giode/i18n"
	"github.com/xmhrnx/middlelan/internal/firewall"
	"github.com/xmhrnx/middlelan/internal/inventory"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// Right-click menus (long-press on touch screens) on apps, devices and
// remote addresses. Each is kept per item and per language, so its
// actions know their target and its labels follow the language.

// contextual is what giode's elements and components implement.
type contextual interface {
	Context(giode.ContextMenu)
}

// withMenu anchors m to el and returns el.
func withMenu(el giode.Element, m giode.ContextMenu) giode.Element {
	if c, ok := el.(contextual); ok {
		c.Context(m)
	}
	return el
}

// newMenu is a menu in the app's colors.
func newMenu() *menu.Menu {
	return giode.Menu(styleMenu).SeparatorColor(giode.Mix("theme.text", "theme.surface", 0.16))
}

func menuKey(id string) [2]any { return [2]any{i18n.Version(), id} }

// copyText puts text on the clipboard and says so.
func (sh *Shell) copyText(text string) {
	sh.svc.App.SetClipboard(text)
	sh.Toast(giode.ToastSuccess, tr("Copied {text}", "text", text))
}

// appMenu is the menu of a program.
func (sh *Shell) appMenu(name string) *menu.Menu {
	return giode.Keyed("app-menu", menuKey(name), func() *menu.Menu {
		m := newMenu().
			Item(tr("Open details"), func() { sh.openApp(name) }).
			Item(tr("Show its connections"), func() {
				sh.connections.showOnly(name)
				sh.navigate(screenConnections)
			})
		if isIOS { // no Connections page
			m = newMenu().Item(tr("Open details"), func() { sh.openApp(name) })
		}
		if fw := sh.svc.Firewall; fw != nil && fw.Capabilities().PerApp {
			m.Separator().CheckItem(tr("Block network access"),
				func() bool { return sh.appBlockRule(name) != "" },
				func() { sh.toggleAppBlock(name) })
		}
		m.Separator().Item(tr("Copy name"), func() { sh.copyText(name) })
		if a, ok := sh.findApp(name); ok && a.Exe != "" {
			exe := a.Exe
			m.Item(tr("Copy program path"), func() { sh.copyText(exe) })
		}
		return m
	})
}

// hostMenu is the menu of a remote address; host is its name, if known.
func (sh *Shell) hostMenu(ip, host string) *menu.Menu {
	return giode.Keyed("host-menu", menuKey(ip+"|"+host), func() *menu.Menu {
		m := newMenu().Item(tr("Copy address"), func() { sh.copyText(ip) })
		if host != "" {
			m.Item(tr("Copy host name"), func() { sh.copyText(host) })
		}
		target := nonEmpty(host, ip)
		m.Separator().
			Item(tr("Ping"), func() { sh.tools.openWith(toolPing, ip) }).
			Item(tr("WHOIS"), func() { sh.tools.openWith(toolWhois, ip) })
		if !isIOS {
			m.Item(tr("Traceroute"), func() { sh.tools.openWith(toolTrace, ip) })
		}
		if host != "" {
			m.Item(tr("DNS lookup"), func() { sh.tools.openWith(toolDNS, target) })
		}
		if fw := sh.svc.Firewall; fw != nil && fw.Capabilities().PerDestination {
			m.Separator().CheckItem(tr("Block this address"),
				func() bool { return sh.destBlockRule(ip) != "" },
				func() {
					if id := sh.destBlockRule(ip); id != "" {
						sh.firewallChange(func(fw *firewall.Firewall) error { return fw.Remove(id) })
						return
					}
					sh.blockRule(firewall.Rule{Dest: ip, Note: tr("Blocked from a connection")})
				})
		}
		return m
	})
}

// deviceMenu is the menu of a device on the local network.
func (sh *Shell) deviceMenu(dev inventory.Device, details func()) *menu.Menu {
	return giode.Keyed("device-menu", menuKey(dev.Key+"|"+dev.IP+"|"+dev.MAC), func() *menu.Menu {
		ip, mac := dev.IP, dev.MAC
		m := newMenu().Item(tr("Details…"), details)
		if ip != "" {
			m.Separator().
				Item(tr("Ping"), func() { sh.tools.openWith(toolPing, ip) }).
				Item(tr("Scan ports"), func() { sh.tools.openWith(toolPorts, ip) })
		}
		if mac != "" {
			m.Item(tr("Wake"), func() {
				if err := nettools.WakeOnLAN(mac, ""); err != nil {
					sh.Toast(giode.ToastError, tr("Wake on LAN failed: {error}", "error", err.Error()))
					return
				}
				sh.Toast(giode.ToastSuccess, tr("Wake-up packet sent to {device}", "device", dev.Name()))
			})
		}
		m.Separator()
		if ip != "" {
			m.Item(tr("Copy address"), func() { sh.copyText(ip) })
		}
		if mac != "" {
			m.Item(tr("Copy MAC address"), func() { sh.copyText(mac) })
		}
		return m
	})
}

// appBlockRule is the ID of the rule blocking a whole program, or "".
func (sh *Shell) appBlockRule(name string) string {
	if sh.svc.Firewall == nil {
		return ""
	}
	for _, r := range sh.svc.Firewall.Rules() {
		if r.App == name && r.Dest == "" {
			return r.ID
		}
	}
	return ""
}

// destBlockRule is the ID of the rule blocking an address for every
// program, or "".
func (sh *Shell) destBlockRule(ip string) string {
	if sh.svc.Firewall == nil {
		return ""
	}
	for _, r := range sh.svc.Firewall.Rules() {
		if r.Dest == ip && r.App == "" {
			return r.ID
		}
	}
	return ""
}

// toggleAppBlock blocks a program, or lifts its block.
func (sh *Shell) toggleAppBlock(name string) {
	if id := sh.appBlockRule(name); id != "" {
		sh.firewallChange(func(fw *firewall.Firewall) error { return fw.Remove(id) })
		return
	}
	a, _ := sh.findApp(name)
	sh.blockRule(firewall.Rule{App: name, Exe: a.Exe, Note: tr("Blocked from the app's page")})
}

// blockedDomainMenu is the menu of a domain ad blocking refused.
func (sh *Shell) blockedDomainMenu(name string) *menu.Menu {
	return giode.Keyed("blocked-domain-menu", menuKey(name), func() *menu.Menu {
		return newMenu().
			Item(tr("Allow"), func() { sh.allowDomain(name) }).
			Item(tr("DNS lookup"), func() { sh.tools.openWith(toolDNS, name) }).
			Separator().
			Item(tr("Copy host name"), func() { sh.copyText(name) })
	})
}
