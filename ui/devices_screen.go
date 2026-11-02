package ui

import (
	"context"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/xmhrnx/middlelan/internal/inventory"
)

// devicesScreen finds and lists the devices on the local network.
type devicesScreen struct {
	shell  *Shell
	scroll *scroll.Scroll
	scan   *giode.ButtonElement
	filter *segmented.Control
	dialog *deviceDialog
}

func newDevicesScreen(sh *Shell) *devicesScreen {
	d := &devicesScreen{shell: sh, dialog: newDeviceDialog(sh)}
	d.scan = giode.Button("Scan network", stylePrimary).Icon(giode.Icon("radar", giode.Styles{Width: 16})).OnClick(func() {
		if running, _, _, _ := sh.svc.Scanner.Status(); running {
			sh.svc.Scanner.Stop()
			return
		}
		sh.svc.Scanner.Start(context.Background())
	})
	d.filter = giode.Input.Segmented("Online", "All")
	d.scroll = giode.Scroll(d.content, giode.Styles{Width: -1, Height: -1, ScrollBar: giode.ScrollBar{AutoHide: true, Width: 6}})
	return d
}

func (d *devicesScreen) view() giode.Element {
	return giode.Box(giode.Styles{Width: -1, Height: -1}, d.scroll, d.dialog.dialog)
}

func (d *devicesScreen) content() giode.Element {
	sc := d.shell.svc.Scanner
	running, done, total, err := sc.Status()
	inv := sc.Inventory()
	now := time.Now()
	all := inv.List()
	online := 0
	for _, dev := range all {
		if dev.Online {
			online++
		}
	}
	if running {
		d.scan.Label("Stop scan")
	} else {
		d.scan.Label("Scan network")
	}
	sub := "Not scanned yet"
	if t := inv.LastScan(); !t.IsZero() {
		sub = plural(online, "device", "devices") + " online · scanned " + formatAgo(t, now)
	}
	kids := []giode.Element{pageHeader("Devices", sub, d.scan)}
	if running {
		p := float32(0)
		if total > 0 {
			p = float32(done) / float32(total)
		}
		kids = append(kids, cardStyled(giode.Styles{Gap: 8},
			giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter},
				giode.Spinner(giode.Styles{Width: 18, Color: "theme.primary"}),
				giode.Text("Scanning your network… "+itoa(done)+" of "+itoa(total)+" addresses", styleBody)),
			giode.Progress(p, giode.Styles{Color: "theme.primary", Background: colGrid, Height: 6}),
		))
	} else if err != nil {
		kids = append(kids, noticeBanner("shield-alert", "Scan problem: "+err.Error(), "theme.warning", nil))
	}
	kids = append(kids, d.networkCard())
	rows := []giode.Element{}
	for _, dev := range all {
		if d.filter.Selected() == 0 && !dev.Online {
			continue
		}
		rows = append(rows, d.devicePressable(dev.Key))
	}
	var list giode.Element = giode.Stack(giode.Styles{Gap: 2, Width: -1}, rows...)
	if len(rows) == 0 {
		list = emptyState("radar", "No devices yet", "Scan to find the phones, computers, TVs and gadgets on your network. MiddleLAN remembers them and tells you when a new one joins.", nil)
	}
	kids = append(kids, card("On this network", d.filter, list))
	return pageBody(kids...)
}

func (d *devicesScreen) networkCard() giode.Element {
	ni := d.shell.netInfo.Load()
	pub := d.shell.public.Load()
	item := func(label, value string) giode.Element {
		if value == "" {
			value = "—"
		}
		return giode.Stack(giode.Styles{Gap: 4},
			giode.Text(label, styleLabel),
			giode.Text(value, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightMedium}),
		)
	}
	var local, gw, subnet, public, isp string
	if ni != nil {
		local, gw, subnet = ni.LocalIP, ni.Gateway, ni.Subnet
	}
	if pub != nil {
		public = pub.IP
		isp = pub.ISP
		if isp == "" {
			isp = pub.Org
		}
	}
	return cardStyled(giode.Styles{},
		responsiveGrid(140, 5, 14, item("This device", local), item("Router", gw), item("Subnet", subnet), item("Public IP", public), item("Provider", isp)))
}

func (d *devicesScreen) devicePressable(key string) giode.Element {
	return giode.Keyed("device-row", key, func() *giode.PressableElement {
		return giode.Pressable(func(s giode.PressState) giode.Element {
			dev, ok := d.find(key)
			if !ok {
				return giode.Box(giode.Styles{})
			}
			return deviceRow(dev, s, time.Now())
		}).Label(key).OnClick(func(giode.ClickEvent) {
			if dev, ok := d.find(key); ok {
				d.dialog.open(dev)
			}
		})
	})
}

func (d *devicesScreen) find(key string) (inventory.Device, bool) {
	for _, dev := range d.shell.svc.Scanner.Inventory().List() {
		if dev.Key == key {
			return dev, true
		}
	}
	return inventory.Device{}, false
}
