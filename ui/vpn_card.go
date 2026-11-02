package ui

import (
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/vpn"
)

// vpnControl is the on-device VPN switch (Android only): with it on,
// MiddleLAN sees every app's connections and domain names and can block
// them, all on the device.
type vpnControl struct {
	shell    *Shell
	sw       *toggle.Switch
	starting bool
}

func newVPNControl(sh *Shell) *vpnControl {
	v := &vpnControl{shell: sh}
	v.sw = switchT(msg("See and filter every app's connections"), giode.Styles{Color: "theme.text", FontSize: sizeBody, Width: -1}, func(on bool) {
		if on {
			v.start()
		} else if err := vpn.Stop(); err != nil {
			sh.Toast(giode.ToastError, tr("Couldn't stop the VPN: {error}", "error", err.Error()))
		}
	})
	return v
}

// start asks for the VPN consent if needed, then starts the service. The
// consent dialog is the system's; this waits for it in the background.
func (v *vpnControl) start() {
	if v.starting {
		return
	}
	v.starting = true
	sh := v.shell
	go func() {
		defer sh.svc.App.Post(func() { v.starting = false })
		granted, err := vpn.Prepare()
		for deadline := time.Now().Add(90 * time.Second); err == nil && !granted && time.Now().Before(deadline); {
			time.Sleep(500 * time.Millisecond)
			granted = vpn.Granted()
		}
		switch {
		case err != nil:
			sh.Toast(giode.ToastError, tr("Couldn't start the VPN: {error}", "error", err.Error()))
		case !granted:
			sh.Toast(giode.ToastInfo, tr("The VPN needs your permission to start."))
		default:
			if err := vpn.Start(); err != nil {
				sh.Toast(giode.ToastError, tr("Couldn't start the VPN: {error}", "error", err.Error()))
			}
		}
		sh.Redraw()
	}()
}

// card is the VPN section of the Security page; nil where there's no VPN.
func (v *vpnControl) card() giode.Element {
	if !vpn.Supported() {
		return nil
	}
	running := vpn.Default.Running()
	if !v.starting {
		v.sw.SetChecked(running)
	}
	kids := []giode.Element{
		v.sw,
		giode.Text(tr("Android hides other apps' connections. This local VPN relays traffic on the phone itself, never through a server, so MiddleLAN can show which app connects where, capture domain names and apply your firewall rules."), styleMuted),
	}
	switch {
	case v.starting:
		kids = append(kids, giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter},
			giode.Spinner(giode.Styles{Width: 14, Color: "theme.primary"}), giode.Text(tr("Starting…"), styleMuted)))
	case running:
		open := 0
		for _, f := range vpn.Default.Flows() {
			if !f.Closed {
				open++
			}
		}
		kids = append(kids, pill(trn("Running · {count} open connections", open), "theme.success"))
	}
	if err := vpn.LastError(); err != nil && !running {
		kids = append(kids, noticeBanner("shield-alert", tr("The VPN stopped: {error}", "error", err.Error()), "theme.warning", nil))
	}
	kids = append(kids, giode.Text(tr("Another VPN app can't run at the same time; Android allows one VPN."), styleMuted))
	return card(tr("On-device VPN"), nil, kids...)
}
