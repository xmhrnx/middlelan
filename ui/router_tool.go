package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// routerTool audits the router's configuration.
type routerTool struct {
	shell    *Shell
	gateway  *input.Input
	run      *giode.ButtonElement
	runner   toolRunner
	findings []nettools.Finding
	err      string
	checked  time.Time
}

func newRouterTool(sh *Shell) *routerTool {
	r := &routerTool{shell: sh, runner: toolRunner{shell: sh}}
	r.gateway = giode.Input.Text("Router address (found automatically)", styleField)
	r.run = runButton("Check router", r.start, &r.runner)
	return r
}

func (r *routerTool) setTarget(host string) { r.gateway.SetText(host) }

func (r *routerTool) start() {
	gw := strings.TrimSpace(r.gateway.Text())
	if gw == "" {
		if ni := r.shell.netInfo.Load(); ni != nil {
			gw = ni.Gateway
			r.gateway.SetText(gw)
		}
	}
	if gw == "" {
		r.err = "The router's address isn't known yet. Type it in, e.g. 192.168.1.1."
		return
	}
	r.findings, r.err = nil, ""
	r.runner.start(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		f, err := nettools.CheckRouter(ctx, gw)
		r.runner.post(func() {
			sort.SliceStable(f, func(i, j int) bool { return f[i].Severity > f[j].Severity })
			r.findings, r.checked = f, time.Now()
			if err != nil && ctx.Err() == nil {
				r.err = err.Error()
			}
		})
	})
}

func (r *routerTool) view(t *toolsScreen, info toolInfo) giode.Element {
	results := []giode.Element{}
	if r.err != "" {
		results = append(results, noticeBanner("shield-alert", r.err, "theme.warning", nil))
	}
	if r.runner.running {
		results = append(results, giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter}, giode.Spinner(giode.Styles{Width: 16, Color: "theme.primary"}), giode.Text("Checking services and DNS…", styleMuted)))
	}
	if len(r.findings) > 0 {
		worst := nettools.SevInfo
		for _, f := range r.findings {
			worst = max(worst, f.Severity)
		}
		score := map[nettools.Severity]string{nettools.SevInfo: "Looks good", nettools.SevLow: "Minor issues", nettools.SevMedium: "Needs attention", nettools.SevHigh: "At risk"}[worst]
		results = append(results, cardStyled(giode.Styles{},
			giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter},
				iconBadge(map[bool]string{true: "shield-check", false: "shield-alert"}[worst <= nettools.SevLow], severityColor(worst), 48),
				giode.Stack(giode.Styles{Gap: 3},
					giode.Text(score, styleHeading),
					giode.Text(plural(len(r.findings), "finding", "findings")+" · checked "+formatAgo(r.checked, time.Now()), styleMuted)))))
		for _, f := range r.findings {
			results = append(results, findingCard(f))
		}
	}
	return t.toolFrame(info, formRow(r.gateway, syncRunLabel(r.run, "Check router", &r.runner)), results...)
}

func severityColor(s nettools.Severity) giode.Color {
	switch s {
	case nettools.SevHigh:
		return "theme.danger"
	case nettools.SevMedium:
		return "theme.warning"
	case nettools.SevLow:
		return "theme.info"
	}
	return "theme.success"
}

func findingCard(f nettools.Finding) giode.Element {
	c := severityColor(f.Severity)
	return giode.Row(giode.Styles{Gap: 14, Padding: giode.UniformInset(16), Background: "theme.surface", BorderRadius: 14, BorderWidth: 1, BorderColor: giode.Mix(c, "theme.surface", 0.4), Width: -1, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{Width: 4, Height: 40, BorderRadius: 2, Background: c}),
		giode.Stack(giode.Styles{Gap: 5, FlexGrow: 1, Width: -1},
			giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter},
				giode.Text(f.Title, giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightSemiBold}),
				pill(f.Severity.String(), c)),
			giode.Text(f.Detail, giode.Styles{Color: "theme.muted", FontSize: sizeSmall}),
		))
}
