package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// uptimeTool pings a reliable host every few seconds, in the background,
// charting latency and logging outages, like a connection watchdog.
type uptimeTool struct {
	shell   *Shell
	host    *input.Input
	run     *giode.ButtonElement
	runner  toolRunner
	samples []uptimeSample
	outages []outage
	started time.Time
	misses  int
}

type uptimeSample struct {
	at  time.Time
	rtt time.Duration
	ok  bool
}

type outage struct {
	from, to time.Time // to is zero while ongoing
}

const (
	uptimeInterval = 3 * time.Second
	uptimeKeep     = 1200 // one hour
	outageAfter    = 3    // consecutive misses that make an outage
)

func newUptimeTool(sh *Shell) *uptimeTool {
	u := &uptimeTool{shell: sh, runner: toolRunner{shell: sh}}
	u.host = textInputT("Host to watch", styleField)
	u.host.SetText("1.1.1.1")
	u.run = runButton("Start monitoring", u.start, &u.runner)
	return u
}

func (u *uptimeTool) setTarget(host string) { u.host.SetText(host) }

func (u *uptimeTool) start() {
	host := strings.TrimSpace(u.host.Text())
	if host == "" {
		return
	}
	u.samples, u.outages, u.started, u.misses = nil, nil, time.Now(), 0
	u.runner.start(func(ctx context.Context) {
		_, _ = nettools.Ping(ctx, host, 0, uptimeInterval, func(r nettools.PingReply) {
			u.runner.post(func() { u.add(r) })
		})
	})
}

func (u *uptimeTool) add(r nettools.PingReply) {
	now := time.Now()
	u.samples = append(u.samples, uptimeSample{at: now, rtt: r.RTT, ok: r.OK})
	if len(u.samples) > uptimeKeep {
		u.samples = u.samples[len(u.samples)-uptimeKeep:]
	}
	if r.OK {
		if u.misses >= outageAfter && len(u.outages) > 0 {
			u.outages[len(u.outages)-1].to = now
			if err := u.shell.svc.App.Notify(tr("Internet is back"), tr("The connection was down for {duration}.", "duration", now.Sub(u.outages[len(u.outages)-1].from).Round(time.Second).String())); err != nil {
				u.shell.Toast(giode.ToastSuccess, tr("Internet is back"))
			}
		}
		u.misses = 0
		return
	}
	u.misses++
	if u.misses == outageAfter {
		u.outages = append(u.outages, outage{from: now.Add(-time.Duration(outageAfter-1) * uptimeInterval)})
		if err := u.shell.svc.App.Notify(tr("Internet connection lost"), tr("MiddleMan can't reach {host}.", "host", u.host.Text())); err != nil {
			u.shell.Toast(giode.ToastError, tr("Internet connection lost"))
		}
	}
}

func (u *uptimeTool) view(t *toolsScreen, info toolInfo) giode.Element {
	results := []giode.Element{}
	if len(u.samples) > 0 {
		ok, sum := 0, time.Duration(0)
		vals := make([]float64, 0, len(u.samples))
		for _, s := range u.samples {
			if s.ok {
				ok++
				sum += s.rtt
			}
			vals = append(vals, float64(s.rtt)/float64(time.Millisecond))
		}
		avg := time.Duration(0)
		if ok > 0 {
			avg = sum / time.Duration(ok)
		}
		upPct := float64(ok) / float64(len(u.samples)) * 100
		state, tone := tr("Online"), giode.Color("theme.success")
		if u.misses >= outageAfter {
			state, tone = tr("Offline"), "theme.danger"
		}
		results = append(results,
			responsiveGrid(150, 4, 12,
				statTile("signal", tr("Status"), state, tr("Watching for {duration}", "duration", time.Since(u.started).Round(time.Second).String()), tone),
				statTile("activity", tr("Average latency"), formatMs(avg), "", "theme.primary"),
				statTile("shield-check", tr("Uptime"), fmt.Sprintf("%.2f%%", upPct), plural(len(u.samples), "check", "checks"), "theme.success"),
				statTile("zap", tr("Outages"), itoa(len(u.outages)), tr("3 missed replies in a row"), "theme.warning"),
			),
			card(tr("Latency"), giode.Text(tr("Every {interval}", "interval", uptimeInterval.String()), styleMuted), sparkline(vals, -1, 130, "theme.primary"), u.lossStrip()),
		)
		if len(u.outages) > 0 {
			rows := []giode.Element{}
			for i := len(u.outages) - 1; i >= 0; i-- {
				o := u.outages[i]
				dur := tr("ongoing")
				if !o.to.IsZero() {
					dur = o.to.Sub(o.from).Round(time.Second).String()
				}
				rows = append(rows, giode.Row(giode.Styles{Gap: 12, Width: -1, Padding: giode.SymmetricInset(2, 6)},
					giode.Icon("zap", giode.Styles{Width: 16, Color: "theme.danger"}),
					giode.Text(o.from.Format("Jan 2 15:04:05"), giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1}),
					giode.Text(dur, styleMuted)))
			}
			results = append(results, card(tr("Outage log"), nil, giode.Stack(giode.Styles{Width: -1}, rows...)))
		}
	}
	form := giode.Stack(giode.Styles{Gap: 10, Width: -1},
		formRow(u.host, syncRunLabel(u.run, "Start monitoring", &u.runner)),
		giode.Text(tr("Keeps running in the background while MiddleMan is open, and notifies you of outages."), styleMuted))
	return t.toolFrame(info, form, results...)
}

// lossStrip marks each check green or red, newest on the right.
func (u *uptimeTool) lossStrip() giode.Element {
	samples := u.samples
	return giode.Canvas(giode.Styles{Width: -1, Height: 10}, func(p *giode.Painter) {
		w := p.Width()
		n := len(samples)
		if n == 0 {
			return
		}
		bw := w / float32(max(n, 120))
		for i, s := range samples {
			c := giode.Color("theme.success")
			if !s.ok {
				c = "theme.danger"
			}
			x := w - float32(n-i)*bw
			p.Rect(x, 0, max(bw-1, 1), 10, 1, c)
		}
	})
}
