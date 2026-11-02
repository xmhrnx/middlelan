package ui

import (
	"fmt"

	"github.com/mehran9675/giode"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// routerCard shows the whole network's internet traffic, every device
// included, from the router's UPnP counters.
func (sh *Shell) routerCard() giode.Element {
	if sh.svc.Router == nil {
		return nil
	}
	st := sh.svc.Router.Status()
	if !st.Found {
		msg := tr("Looking for your router…")
		if st.Err != "" {
			msg = tr("No router answered over UPnP. To see every device's internet traffic here, turn on UPnP in your router's settings.")
		}
		return noticeBanner("router", msg, "theme.info", nil)
	}
	points := make([]monitor.RatePoint, len(st.History))
	for i, p := range st.History {
		points[i] = monitor.RatePoint{Time: p.At, Rx: p.Rx, Tx: p.Tx}
	}
	name := nonEmpty(st.Name, tr("Your router"))
	line := ""
	if st.LinkDown > 0 {
		line = tr("Line {down} / {up} Mbps", "down", fmt.Sprintf("%.0f", st.LinkDown), "up", fmt.Sprintf("%.0f", st.LinkUp))
	}
	return card(tr("Whole network · {router}", "router", name), giode.Text(line, styleMuted),
		ratePair(formatRate(st.RateRx), formatRate(st.RateTx), 14),
		rateChart(points, 120),
		giode.Text(tr("Every device's internet traffic, as your router counts it."), styleMuted))
}
