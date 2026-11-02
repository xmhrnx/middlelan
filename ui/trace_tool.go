package ui

import (
	"context"
	"strings"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// traceTool shows the route to a host, hop by hop.
type traceTool struct {
	shell  *Shell
	host   *input.Input
	run    *giode.ButtonElement
	runner toolRunner
	hops   []nettools.Hop
	err    string
	done   bool
}

func newTraceTool(sh *Shell) *traceTool {
	t := &traceTool{shell: sh, runner: toolRunner{shell: sh}}
	t.host = giode.Input.Text("Host or IP, e.g. example.com", styleField).Keyboard(giode.KeyboardURL)
	t.host.SetText("1.1.1.1")
	t.host.Submit(func(string) { t.start() })
	t.run = runButton("Trace", t.start, &t.runner)
	return t
}

func (t *traceTool) setTarget(host string) { t.host.SetText(host) }

func (t *traceTool) start() {
	host := strings.TrimSpace(t.host.Text())
	if host == "" {
		return
	}
	t.hops, t.err, t.done = nil, "", false
	t.runner.start(func(ctx context.Context) {
		_, err := nettools.Traceroute(ctx, host, 30, func(h nettools.Hop) {
			t.runner.post(func() { t.hops = append(t.hops, h) })
		})
		t.runner.post(func() {
			t.done = true
			if err != nil && ctx.Err() == nil {
				t.err = err.Error()
			}
		})
	})
}

func (t *traceTool) view(ts *toolsScreen, info toolInfo) giode.Element {
	results := []giode.Element{}
	if t.err != "" {
		results = append(results, noticeBanner("shield-alert", t.err, "theme.danger", nil))
	}
	if len(t.hops) > 0 || t.runner.running {
		rows := []giode.Element{}
		for _, h := range t.hops {
			rows = append(rows, t.hopRow(h))
		}
		if t.runner.running {
			rows = append(rows, giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Padding: giode.UniformInset(8)},
				giode.Spinner(giode.Styles{Width: 16, Color: "theme.primary"}), giode.Text("Tracing…", styleMuted)))
		}
		results = append(results, card("Route", nil, giode.Stack(giode.Styles{Width: -1}, rows...)))
	}
	return ts.toolFrame(info, formRow(t.host, syncRunLabel(t.run, "Trace", &t.runner)), results...)
}

func (t *traceTool) hopRow(h nettools.Hop) giode.Element {
	dot := giode.Color("theme.primary")
	name, sub := h.Host, h.Addr
	switch {
	case h.Timeout:
		dot, name, sub = "theme.muted", "No reply", "This router doesn't answer probes"
	case h.Reached:
		dot = "theme.success"
	}
	if name == "" {
		name = h.Addr
		if info, ok := t.shell.svc.Resolver.Peek(h.Addr); ok && info.Owner() != "" {
			sub = h.Addr + " · " + info.Owner()
		}
	}
	rtt := ""
	if !h.Timeout {
		rtt = formatMs(h.RTT)
	}
	return giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(4, 7), Width: -1, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{Width: 28, Height: 28, BorderRadius: 14, Background: giode.Mix(dot, "theme.surface", 0.18), Align: giode.AlignCenter, Justify: giode.JustifyCenter},
			giode.Text(itoa(h.TTL), giode.Styles{Color: dot, FontSize: sizeTiny, FontWeight: giode.FontWeightBold})),
		giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1, Width: -1},
			giode.Text(name, giode.Styles{Color: "theme.text", FontSize: sizeBody, TextOverflow: giode.TextOverflowEllipsis}),
			giode.Text(sub, giode.Styles{Color: "theme.muted", FontSize: sizeTiny, FontFamily: fontMono}),
		),
		giode.Text(rtt, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontWeight: giode.FontWeightMedium}),
	)
}
