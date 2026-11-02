package ui

import (
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dialog"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/inventory"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// deviceDialog shows a device's details and lets the user name it, trust
// it, and run tools against it.
type deviceDialog struct {
	shell   *Shell
	isOpen  bool
	dev     inventory.Device
	dialog  *dialog.Dialog
	label   *input.Input
	trusted *toggle.Switch
	save    *giode.ButtonElement
	ping    *giode.ButtonElement
	ports   *giode.ButtonElement
	wake    *giode.ButtonElement
	forget  *giode.ButtonElement
}

func newDeviceDialog(sh *Shell) *deviceDialog {
	dd := &deviceDialog{shell: sh}
	dd.label = giode.Input.Text("Name this device", styleField)
	dd.trusted = giode.Input.Switch("Known device (no alerts)", giode.Styles{Color: "theme.text", Width: -1})
	dd.save = giode.Button("Save", stylePrimary).OnClick(dd.saveChanges)
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})
	dd.ping = giode.Button("Ping", small).Variant(giode.VariantOutlined).Icon(giode.Icon("activity", giode.Styles{Width: 14})).OnClick(func() {
		dd.isOpen = false
		sh.tools.openWith(toolPing, dd.dev.IP)
	})
	dd.ports = giode.Button("Scan ports", small).Variant(giode.VariantOutlined).Icon(giode.Icon("scan-search", giode.Styles{Width: 14})).OnClick(func() {
		dd.isOpen = false
		sh.tools.openWith(toolPorts, dd.dev.IP)
	})
	dd.wake = giode.Button("Wake", small).Variant(giode.VariantOutlined).Icon(giode.Icon("power", giode.Styles{Width: 14})).OnClick(func() {
		if err := nettools.WakeOnLAN(dd.dev.MAC, ""); err != nil {
			sh.Toast(giode.ToastError, "Wake on LAN failed: "+err.Error())
			return
		}
		sh.Toast(giode.ToastSuccess, "Wake-up packet sent to "+dd.dev.Name())
	})
	dd.forget = giode.Button("Forget", small.With(giode.Styles{Color: "theme.danger", BorderColor: colDangerSoft})).Variant(giode.VariantOutlined).OnClick(func() {
		if err := sh.svc.Scanner.Inventory().Forget(dd.dev.Key); err != nil {
			sh.Toast(giode.ToastError, err.Error())
		}
		dd.isOpen = false
	})
	dd.dialog = giode.Dialog(&dd.isOpen, dd.content, giode.Styles{Background: "theme.surface", BorderRadius: 18, Width: 460})
	return dd
}

func (dd *deviceDialog) open(d inventory.Device) {
	dd.dev = d
	dd.label.SetText(d.Label)
	dd.trusted.SetChecked(d.Trusted)
	dd.isOpen = true
}

func (dd *deviceDialog) saveChanges() {
	label, trusted := strings.TrimSpace(dd.label.Text()), dd.trusted.Checked()
	err := dd.shell.svc.Scanner.Inventory().Update(dd.dev.Key, func(d *inventory.Device) {
		d.Label, d.Trusted = label, trusted
	})
	if err != nil {
		dd.shell.Toast(giode.ToastError, "Couldn't save: "+err.Error())
		return
	}
	dd.isOpen = false
}

func (dd *deviceDialog) content() giode.Element {
	d := dd.dev
	fact := func(label, value string) giode.Element {
		if value == "" {
			value = "—"
		}
		return giode.Row(giode.Styles{Gap: 10, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Text(label, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, Width: 110}),
			giode.Text(value, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1}),
		)
	}
	ports := []string{}
	for _, p := range d.OpenPorts {
		ports = append(ports, serviceLabel(p))
	}
	actions := []giode.Element{dd.ping, dd.ports}
	if d.MAC != "" && !d.IsSelf {
		actions = append(actions, dd.wake)
	}
	if !d.IsSelf {
		actions = append(actions, dd.forget)
	}
	return giode.Stack(giode.Styles{Gap: 14, Width: -1},
		giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter},
			iconBadge(deviceIcon(d.Kind), "theme.primary", 48),
			giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1},
				giode.Text(d.Name(), styleHeading),
				giode.Text(deviceKindLabel(d.Kind), styleMuted),
			),
		),
		giode.Stack(giode.Styles{Gap: 6, Width: -1},
			fact("IP address", d.IP), fact("MAC address", strings.ToUpper(d.MAC)), fact("Maker", d.Vendor),
			fact("Host name", d.Hostname), fact("Open ports", strings.Join(ports, ", ")),
			fact("First seen", d.FirstSeen.Local().Format(time.DateTime)), fact("Last seen", d.LastSeen.Local().Format(time.DateTime)),
		),
		giode.Row(giode.Styles{Gap: 8}, actions...),
		giode.Divider(giode.Styles{Color: "theme.border"}),
		dd.label,
		dd.trusted,
		giode.Row(giode.Styles{Justify: giode.JustifyEnd, Width: -1}, dd.save),
	)
}
