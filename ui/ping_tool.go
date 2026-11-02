package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/toggle"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// pingTool measures latency and loss to a host.
type pingTool struct {
	shell      *Shell
	host       *input.Input
	continuous *toggle.Switch
	run        *giode.ButtonElement
	runner     toolRunner
	replies    []nettools.PingReply
	stats      *nettools.PingStats
	err        string
}

func newPingTool(sh *Shell) *pingTool {
	p := &pingTool{shell: sh, runner: toolRunner{shell: sh}}
	p.host = giode.Input.Text("Host or IP, e.g. 1.1.1.1", styleField).Keyboard(giode.KeyboardURL)
	p.host.SetText("1.1.1.1")
	p.host.Submit(func(string) { p.start() })
	p.continuous = giode.Input.Switch("Keep pinging", giode.Styles{Color: "theme.text", Width: -1})
	p.run = runButton("Ping", p.start, &p.runner)
	return p
}

func (p *pingTool) setTarget(host string) { p.host.SetText(host) }

func (p *pingTool) start() {
	host := strings.TrimSpace(p.host.Text())
	if host == "" {
		return
	}
	count := 10
	if p.continuous.Checked() {
		count = 0
	}
	p.replies, p.stats, p.err = nil, nil, ""
	p.runner.start(func(ctx context.Context) {
		stats, err := nettools.Ping(ctx, host, count, time.Second, func(r nettools.PingReply) {
			p.runner.post(func() {
				p.replies = append(p.replies, r)
				if len(p.replies) > 300 {
					p.replies = p.replies[len(p.replies)-300:]
				}
			})
		})
		p.runner.post(func() {
			p.stats = &stats
			if err != nil && ctx.Err() == nil {
				p.err = err.Error()
			}
		})
	})
}

func (p *pingTool) view(t *toolsScreen, info toolInfo) giode.Element {
	form := giode.Stack(giode.Styles{Gap: 12, Width: -1},
		formRow(p.host, syncRunLabel(p.run, "Ping", &p.runner)),
		p.continuous,
	)
	results := []giode.Element{}
	if p.err != "" {
		results = append(results, noticeBanner("shield-alert", p.err, "theme.danger", nil))
	}
	if len(p.replies) > 0 || p.stats != nil {
		results = append(results, p.summary(), p.chart(), p.log())
	}
	return t.toolFrame(info, form, results...)
}

func (p *pingTool) summary() giode.Element {
	var sent, recv int
	var minRTT, maxRTT, sum time.Duration
	for _, r := range p.replies {
		sent++
		if !r.OK {
			continue
		}
		recv++
		sum += r.RTT
		if minRTT == 0 || r.RTT < minRTT {
			minRTT = r.RTT
		}
		maxRTT = max(maxRTT, r.RTT)
	}
	avg := time.Duration(0)
	if recv > 0 {
		avg = sum / time.Duration(recv)
	}
	loss := 0.0
	if sent > 0 {
		loss = float64(sent-recv) / float64(sent) * 100
	}
	method := ""
	if p.stats != nil {
		method = "via " + p.stats.Method
	}
	return responsiveGrid(150, 4, 12,
		statTile("activity", "Average", formatMs(avg), method, "theme.primary"),
		statTile("arrow-down", "Fastest", formatMs(minRTT), "", "theme.success"),
		statTile("arrow-up", "Slowest", formatMs(maxRTT), "", "theme.warning"),
		statTile("signal", "Packet loss", fmt.Sprintf("%.0f%%", loss), fmt.Sprintf("%d of %d replied", recv, sent), "theme.danger"),
	)
}

func (p *pingTool) chart() giode.Element {
	vals := make([]float64, 0, len(p.replies))
	for _, r := range p.replies {
		vals = append(vals, float64(r.RTT)/float64(time.Millisecond))
	}
	return card("Latency", nil, sparkline(vals, -1, 120, "theme.primary"))
}

func (p *pingTool) log() giode.Element {
	rows := []giode.Element{}
	for i := len(p.replies) - 1; i >= 0 && len(rows) < 30; i-- {
		r := p.replies[i]
		line := fmt.Sprintf("#%d  %s  time=%s  ttl=%d", r.Seq, r.Addr, formatMs(r.RTT), r.TTL)
		c := giode.Color("theme.text")
		if !r.OK {
			line, c = fmt.Sprintf("#%d  %s", r.Seq, nonEmpty(r.Err, "timed out")), "theme.danger"
		}
		rows = append(rows, giode.Text(line, giode.Styles{Color: c, FontSize: sizeSmall, FontFamily: fontMono}))
	}
	return card("Replies", nil, giode.Stack(giode.Styles{Gap: 4}, rows...))
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
