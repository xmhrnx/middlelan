package ui

import (
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dialog"
	"github.com/xmhrnx/middlelan/internal/firewall"
)

// firewallPrompt asks what to do when a program connects for the first
// time, while the firewall is on and the user opted in. Programs queue up
// so several new ones are asked about one at a time.
type firewallPrompt struct {
	shell  *Shell
	open   bool
	queue  []string
	dialog *dialog.Dialog
	allow  *giode.ButtonElement
	block  *giode.ButtonElement
}

func newFirewallPrompt(sh *Shell) *firewallPrompt {
	p := &firewallPrompt{shell: sh}
	p.allow = buttonT("Allow", styleOutline).Variant(giode.VariantOutlined).OnClick(p.next)
	p.block = buttonT("Block", stylePrimary.With(giode.Styles{Background: "theme.danger", Color: "#ffffff"})).OnClick(func() {
		if len(p.queue) > 0 {
			name := p.queue[0]
			exe := ""
			if a, ok := sh.findApp(name); ok {
				exe = a.Exe
			}
			sh.blockRule(firewall.Rule{App: name, Exe: exe, Note: tr("Blocked when it first connected")})
		}
		p.next()
	})
	p.dialog = giode.Dialog(&p.open, p.content, giode.Styles{Background: "theme.surface", BorderRadius: 18, Width: 440}).Dismissible(false)
	return p
}

// AskAboutApp queues a program for the first-connection prompt; safe
// from any goroutine.
func (sh *Shell) AskAboutApp(name string) {
	sh.svc.App.Post(func() {
		p := sh.prompt
		for _, q := range p.queue {
			if q == name {
				return
			}
		}
		p.queue = append(p.queue, name)
		p.open = true
		sh.svc.App.ShowWindow()
	})
}

func (p *firewallPrompt) next() {
	if len(p.queue) > 0 {
		p.queue = p.queue[1:]
	}
	p.open = len(p.queue) > 0
}

func (p *firewallPrompt) content() giode.Element {
	if len(p.queue) == 0 {
		return giode.Box(giode.Styles{})
	}
	name := p.queue[0]
	sub := tr("It's using the network for the first time.")
	if a, ok := p.shell.findApp(name); ok {
		if len(a.Endpoints) > 0 {
			info := p.shell.svc.Resolver.Get(a.Endpoints[0].IP)
			sub = tr("It's connecting to {host}.", "host", info.Name())
			if pl := placeName(info.Place()); pl != "" {
				sub = tr("It's connecting to {host} ({place}).", "host", info.Name(), "place", pl)
			}
		}
		if a.Exe != "" {
			sub += "\n" + a.Exe
		}
	}
	more := ""
	if len(p.queue) > 1 {
		more = trn("{count} more programs waiting", len(p.queue)-1)
	}
	return giode.Stack(giode.Styles{Gap: 14, Width: -1},
		giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter},
			giode.Avatar(name, giode.Styles{Width: 44}),
			giode.Stack(giode.Styles{Gap: 3}, giode.Text(name, styleHeading), giode.Text(tr("New program on the network"), styleMuted))),
		giode.Text(sub, giode.Styles{Color: "theme.text", FontSize: sizeSmall}),
		giode.Text(tr("Blocking adds a firewall rule you can remove in Security at any time."), styleMuted),
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Width: -1},
			giode.Text(more, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FlexGrow: 1}), p.allow, p.block),
	)
}
