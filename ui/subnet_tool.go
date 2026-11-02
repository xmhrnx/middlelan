package ui

import (
	"strconv"
	"strings"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// subnetTool calculates subnet details as the user types.
type subnetTool struct {
	shell *Shell
	cidr  *input.Input
}

func newSubnetTool(sh *Shell) *subnetTool {
	s := &subnetTool{shell: sh}
	s.cidr = giode.Input.Text("CIDR, e.g. 192.168.1.0/24 or 10.0.0.1 255.255.0.0", styleField)
	s.cidr.SetText("192.168.1.0/24")
	return s
}

func (s *subnetTool) setTarget(v string) { s.cidr.SetText(v) }

func (s *subnetTool) view(t *toolsScreen, info toolInfo) giode.Element {
	in := strings.TrimSpace(s.cidr.Text())
	results := []giode.Element{}
	if in != "" {
		sn, err := nettools.CalcSubnet(in)
		if err != nil {
			results = append(results, noticeBanner("info", err.Error(), "theme.warning", nil))
		} else {
			fact := func(label, value string) giode.Element {
				return giode.Stack(giode.Styles{Gap: 4},
					giode.Text(label, styleLabel),
					giode.Text(nonEmpty(value, "—"), giode.Styles{Color: "theme.text", FontSize: 15, FontWeight: giode.FontWeightMedium, FontFamily: fontMono}).Selectable("subnet-"+label))
			}
			scope := "Public"
			if sn.Private {
				scope = "Private"
			}
			results = append(results, cardStyled(giode.Styles{}, responsiveGrid(180, 3, 18,
				fact("Network", sn.Network+"/"+strconv.Itoa(sn.Prefix)), fact("Netmask", sn.Netmask), fact("Wildcard", sn.Wildcard),
				fact("First host", sn.FirstHost), fact("Last host", sn.LastHost), fact("Broadcast", sn.Broadcast),
				fact("Usable hosts", strconv.FormatUint(sn.Hosts, 10)), fact("Class", sn.Class), fact("Scope", scope),
			)))
		}
	}
	return t.toolFrame(info, s.cidr, results...)
}
