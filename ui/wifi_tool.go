package ui

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/wifi"
)

// wifiTool shows the Wi-Fi link (live signal), nearby networks, how
// crowded each channel is, and warns about weak security.
type wifiTool struct {
	shell   *Shell
	runner  toolRunner
	link    *wifi.Link
	linkErr error
	nets    []wifi.Network
	scanErr error
	signal  []float64
	rescan  *giode.ButtonElement
	grant   *giode.ButtonElement
	scanned time.Time
	// viewed is when the page was last drawn; polling stops soon after the
	// user leaves it (or hides the window), since each poll runs a system
	// command.
	viewed atomic.Int64
}

func newWifiTool(sh *Shell) *wifiTool {
	w := &wifiTool{shell: sh, runner: toolRunner{shell: sh}}
	small := styleOutline.With(giode.Styles{Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})
	w.rescan = buttonT("Scan again", small).Variant(giode.VariantOutlined).Icon(giode.Icon("refresh-cw", giode.Styles{Width: 14})).OnClick(w.start)
	w.grant = buttonT("Allow location access", stylePrimary.With(giode.Styles{Padding: giode.SymmetricInset(12, 7), FontSize: sizeSmall})).OnClick(func() {
		if err := requestLocationPermission(); err != nil {
			sh.Toast(giode.ToastError, err.Error())
		}
	})
	return w
}

func (w *wifiTool) setTarget(string) {}

// start polls the link every 3 seconds and scans every 30 while the page
// is open (the runner stops when the user leaves or presses Stop).
func (w *wifiTool) start() {
	w.runner.start(func(ctx context.Context) {
		var lastScan time.Time
		for {
			lctx, cancel := context.WithTimeout(ctx, 8*time.Second)
			l, err := wifi.Current(lctx)
			cancel()
			var nets []wifi.Network
			var serr error
			scanned := time.Since(lastScan) >= 30*time.Second
			if scanned {
				sctx, cancel := context.WithTimeout(ctx, 12*time.Second)
				nets, serr = wifi.Scan(sctx)
				cancel()
				lastScan = time.Now()
			}
			w.runner.post(func() {
				w.link, w.linkErr = &l, err
				if err == nil && l.Connected {
					w.signal = append(w.signal, float64(l.SignalDBm+100))
					if len(w.signal) > 100 {
						w.signal = w.signal[len(w.signal)-100:]
					}
				}
				if scanned {
					w.nets, w.scanErr, w.scanned = nets, serr, time.Now()
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			if time.Since(time.Unix(0, w.viewed.Load())) > 10*time.Second {
				return
			}
		}
	})
}

func (w *wifiTool) view(t *toolsScreen, info toolInfo) giode.Element {
	w.viewed.Store(time.Now().UnixNano())
	if !w.runner.running {
		w.start()
	}
	results := []giode.Element{}
	if errors.Is(w.linkErr, wifi.ErrNeedsLocation) || errors.Is(w.scanErr, wifi.ErrNeedsLocation) {
		var action giode.Element
		if isMobile {
			action = w.grant
		}
		results = append(results, noticeBanner("lock", tr("Your system only shows Wi-Fi network names to apps with location access (and location turned on)."), "theme.info", action))
	}
	results = append(results, w.linkCard(), w.networksCard(), w.channelsCard())
	return t.toolFrame(info, nil, results...)
}

func (w *wifiTool) linkCard() giode.Element {
	switch {
	case w.link == nil:
		return card(tr("This connection"), nil, giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter}, giode.Spinner(giode.Styles{Width: 14, Color: "theme.primary"}), giode.Text(tr("Reading Wi-Fi…"), styleMuted)))
	case errors.Is(w.linkErr, wifi.ErrNotWiFi):
		return card(tr("This connection"), nil, emptyState("wifi", tr("Not on Wi-Fi"), tr("This device is connected by cable, or Wi-Fi is off."), nil))
	case errors.Is(w.linkErr, wifi.ErrUnsupported):
		return card(tr("This connection"), nil, emptyState("wifi", tr("Wi-Fi details aren't available"), tr("This system doesn't report Wi-Fi details to apps."), nil))
	case w.linkErr != nil && !errors.Is(w.linkErr, wifi.ErrNeedsLocation):
		return noticeBanner("shield-alert", tr("Couldn't read Wi-Fi: {error}", "error", w.linkErr.Error()), "theme.warning", nil)
	}
	l := *w.link
	kids := []giode.Element{}
	if weak, why := wifi.Weak(l.Security); weak {
		kids = append(kids, noticeBanner("shield-alert", tr(why), "theme.danger", nil))
	}
	q := l.Quality
	tone := giode.Color("theme.success")
	switch {
	case q < 40:
		tone = "theme.danger"
	case q < 65:
		tone = "theme.warning"
	}
	kids = append(kids,
		responsiveGrid(150, 4, 12,
			statTile("wifi", tr("Network"), nonEmpty(l.SSID, tr("Hidden name")), l.BSSID, "theme.primary"),
			statTile("signal", tr("Signal"), fmt.Sprintf("%d dBm", l.SignalDBm), tr("{percent}% quality", "percent", q), tone),
			statTile("radio", tr("Channel"), itoa(l.Channel), nonEmpty(l.Band, "—"), "theme.accent"),
			statTile("lock", tr("Security"), string(l.Security), rateText(l), "theme.info"),
		),
		giode.Text(tr("Signal over time"), styleLabel),
		sparkline(w.signal, -1, 80, tone),
	)
	return card(tr("This connection"), nil, kids...)
}

// weakReasons are wifi.Weak's explanations, translated where shown.
var _ = []string{
	msg("Open network: traffic over the air isn't encrypted, so anyone nearby can read it."),
	msg("WEP encryption is broken and can be cracked in minutes; switch the router to WPA2 or WPA3."),
	msg("WPA (TKIP) is deprecated and has known attacks; switch the router to WPA2 or WPA3."),
}

func rateText(l wifi.Link) string {
	if l.TxRateMbps > 0 {
		return tr("Link {rate} Mbps", "rate", fmt.Sprintf("%.0f", l.TxRateMbps))
	}
	return ""
}

func (w *wifiTool) networksCard() giode.Element {
	switch {
	case errors.Is(w.scanErr, wifi.ErrNotWiFi), errors.Is(w.scanErr, wifi.ErrUnsupported):
		return nil // the connection card already says why
	case w.scanErr != nil && !errors.Is(w.scanErr, wifi.ErrNeedsLocation):
		return card(tr("Nearby networks"), w.rescan, giode.Text(tr("Couldn't scan: {error}", "error", w.scanErr.Error()), styleMuted))
	}
	rows := []giode.Element{}
	for i, n := range w.nets {
		if i == 25 {
			break
		}
		badge := pill(string(n.Security), "theme.muted")
		if weak, _ := wifi.Weak(n.Security); weak {
			badge = pill(string(n.Security), "theme.danger")
		}
		name := nonEmpty(n.SSID, tr("Hidden network"))
		if n.Connected {
			name += "  ·  " + tr("connected")
		}
		rows = append(rows, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Padding: giode.SymmetricInset(2, 6), Wrap: giode.WrapNoWrap},
			signalBars(n.Quality),
			giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1, Width: -1},
				giode.Text(name, giode.Styles{Color: "theme.text", FontSize: sizeBody, TextOverflow: giode.TextOverflowEllipsis}),
				giode.Text(tr("{band} · channel {channel} · {signal} dBm", "band", nonEmpty(n.Band, "?"), "channel", n.Channel, "signal", n.SignalDBm), styleMuted)),
			badge))
	}
	if len(rows) == 0 {
		rows = append(rows, giode.Text(tr("Scanning…"), styleMuted))
	}
	sub := w.rescan
	return card(tr("Nearby networks"), sub, giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...))
}

func (w *wifiTool) channelsCard() giode.Element {
	if len(w.nets) == 0 {
		return nil
	}
	uses, best := wifi.Congestion(w.nets)
	rows := []giode.Element{}
	peak := 1
	for _, u := range uses {
		peak = max(peak, u.Networks)
	}
	for _, u := range uses {
		rows = append(rows, giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 90}, giode.Text(fmt.Sprintf("%s · %d", u.Band, u.Channel), styleMuted)),
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, shareBar(float64(u.Networks)/float64(peak))),
			giode.Box(giode.Styles{Width: 90}, giode.Text(plural(u.Networks, "network", "networks"), styleMuted))))
	}
	tips := []giode.Element{}
	for band, ch := range best {
		tips = append(tips, pill(tr("Best {band} channel: {channel}", "band", band, "channel", ch), "theme.success"))
	}
	return card(tr("Channel congestion"), nil,
		giode.Row(giode.Styles{Gap: 8}, tips...),
		giode.Stack(giode.Styles{Gap: 6, Width: -1}, rows...),
		giode.Text(tr("Set your router to the least crowded channel for fewer slowdowns."), styleMuted))
}

// signalBars draws four bars filled by quality (0..100).
func signalBars(quality int) giode.Element {
	return giode.Canvas(giode.Styles{Width: 22, Height: 18}, func(p *giode.Painter) {
		lit := (quality + 24) / 25
		for i := 0; i < 4; i++ {
			h := float32(5 + i*4)
			c := colGrid
			if i < lit {
				c = "theme.primary"
			}
			p.Rect(float32(i*6), 18-h, 4, h, 1.5, c)
		}
	})
}
