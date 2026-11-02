package ui

import (
	"strings"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// wakeTool sends Wake-on-LAN packets.
type wakeTool struct {
	shell     *Shell
	mac, bcst *input.Input
	send      *giode.ButtonElement
	pickers   []*giode.PressableElement
}

func newWakeTool(sh *Shell) *wakeTool {
	w := &wakeTool{shell: sh}
	w.mac = giode.Input.Text("MAC address, e.g. 3C:22:FB:12:34:56", styleField)
	w.bcst = giode.Input.Text("Broadcast address (optional)", styleField)
	w.send = giode.Button("Send", stylePrimary).Icon(giode.Icon("power", giode.Styles{Width: 16})).OnClick(func() {
		mac := strings.TrimSpace(w.mac.Text())
		if err := nettools.WakeOnLAN(mac, strings.TrimSpace(w.bcst.Text())); err != nil {
			sh.Toast(giode.ToastError, err.Error())
			return
		}
		sh.Toast(giode.ToastSuccess, "Magic packet sent to "+mac)
	})
	return w
}

func (w *wakeTool) setTarget(mac string) { w.mac.SetText(mac) }

func (w *wakeTool) view(t *toolsScreen, info toolInfo) giode.Element {
	rows := []giode.Element{}
	for _, d := range w.shell.svc.Scanner.Inventory().List() {
		if d.MAC == "" || d.IsSelf {
			continue
		}
		d := d
		rows = append(rows, giode.Keyed("wol-pick", d.Key, func() *giode.PressableElement {
			return giode.Pressable(func(s giode.PressState) giode.Element {
				bg := giode.Color("transparent")
				if s.Hovered {
					bg = colHover
				}
				return giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Padding: giode.SymmetricInset(8, 7), Background: bg, BorderRadius: 10, Width: -1},
					giode.Icon(deviceIcon(d.Kind), giode.Styles{Width: 18, Color: "theme.muted"}),
					giode.Text(d.Name(), giode.Styles{Color: "theme.text", FontSize: sizeBody, FlexGrow: 1}),
					giode.Text(strings.ToUpper(d.MAC), giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FontFamily: fontMono}),
				)
			}).OnClick(func(giode.ClickEvent) { w.mac.SetText(d.MAC) })
		}))
	}
	results := []giode.Element{}
	if len(rows) > 0 {
		results = append(results, card("Known devices", giode.Text("Tap to fill in", styleMuted), giode.Stack(giode.Styles{Width: -1}, rows...)))
	}
	form := giode.Stack(giode.Styles{Gap: 12, Width: -1}, formRow(w.mac, w.send), w.bcst,
		giode.Text("The computer must have Wake-on-LAN enabled in its firmware and be wired to the network.", styleMuted))
	return t.toolFrame(info, form, results...)
}
