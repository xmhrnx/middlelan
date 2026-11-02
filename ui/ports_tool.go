package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// portsTool finds open TCP ports on a host.
type portsTool struct {
	shell        *Shell
	host, custom *input.Input
	preset       *segmented.Control
	run          *giode.ButtonElement
	runner       toolRunner
	open         []nettools.PortResult
	done, total  int
	err          string
}

func newPortsTool(sh *Shell) *portsTool {
	p := &portsTool{shell: sh, runner: toolRunner{shell: sh}}
	p.host = textInputT("Host or IP, e.g. 192.168.1.1", styleField).Keyboard(giode.KeyboardURL)
	p.host.Submit(func(string) { p.start() })
	p.custom = textInputT("Ports, e.g. 22,80,8000-8100", styleField)
	p.preset = segmentedT("Common", "1–1024", "Custom")
	p.run = runButton("Scan", p.start, &p.runner)
	return p
}

func (p *portsTool) setTarget(host string) { p.host.SetText(host) }

func (p *portsTool) start() {
	host := strings.TrimSpace(p.host.Text())
	if host == "" {
		if ni := p.shell.netInfo.Load(); ni != nil {
			host = ni.Gateway
			p.host.SetText(host)
		}
		if host == "" {
			return
		}
	}
	var ports []int
	switch p.preset.Selected() {
	case 0:
		ports = nettools.CommonPorts
	case 1:
		ports, _ = nettools.ParsePortSpec("1-1024")
	default:
		var err error
		if ports, err = nettools.ParsePortSpec(p.custom.Text()); err != nil {
			p.err = err.Error()
			return
		}
	}
	p.open, p.done, p.total, p.err = nil, 0, len(ports), ""
	p.runner.start(func(ctx context.Context) {
		err := nettools.ScanPorts(ctx, host, ports, 48, 900*time.Millisecond, func(r nettools.PortResult) {
			p.runner.post(func() {
				p.open = append(p.open, r)
				sort.Slice(p.open, func(i, j int) bool { return p.open[i].Port < p.open[j].Port })
			})
		}, func(done, total int) {
			if done%8 == 0 || done == total {
				p.runner.post(func() { p.done, p.total = done, total })
			}
		})
		if err != nil && ctx.Err() == nil {
			p.runner.post(func() { p.err = err.Error() })
		}
	})
}

func (p *portsTool) view(t *toolsScreen, info toolInfo) giode.Element {
	formKids := []giode.Element{formRow(p.host, syncRunLabel(p.run, "Scan", &p.runner)), p.preset}
	if p.preset.Selected() == 2 {
		formKids = append(formKids, p.custom)
	}
	formKids = append(formKids, giode.Text(tr("Only scan devices you own or are allowed to test."), styleMuted))
	results := []giode.Element{}
	if p.err != "" {
		results = append(results, noticeBanner("shield-alert", p.err, "theme.danger", nil))
	}
	if p.total > 0 && (p.runner.running || p.done > 0) {
		prog := float32(p.done) / float32(max(p.total, 1))
		rows := []giode.Element{
			giode.Text(tr("{done} of {total} ports checked", "done", p.done, "total", p.total)+" · "+plural(len(p.open), "open port", "open ports"), styleMuted),
			giode.Progress(prog, giode.Styles{Color: "theme.danger", Background: colGrid, Height: 6}),
		}
		for _, r := range p.open {
			rows = append(rows, portRow(r))
		}
		results = append(results, card(tr("Open ports"), nil, giode.Stack(giode.Styles{Gap: 8, Width: -1}, rows...)))
	}
	return t.toolFrame(info, giode.Stack(giode.Styles{Gap: 12, Width: -1}, formKids...), results...)
}

func portRow(r nettools.PortResult) giode.Element {
	svc := r.Service
	if svc == "" {
		svc = tr("Unknown service")
	}
	kids := []giode.Element{
		giode.Box(giode.Styles{Width: 64}, giode.Text(itoa(r.Port), giode.Styles{Color: "theme.text", FontSize: sizeBody, FontWeight: giode.FontWeightBold, FontFamily: fontMono})),
		giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1, Width: -1},
			giode.Text(svc, giode.Styles{Color: "theme.text", FontSize: sizeBody}),
			giode.Text(strings.TrimSpace(r.Banner), giode.Styles{Color: "theme.muted", FontSize: sizeTiny, FontFamily: fontMono, TextOverflow: giode.TextOverflowEllipsis}),
		),
		giode.Text(formatMs(r.Latency), styleMuted),
	}
	return giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(10, 8), Width: -1, Background: colSurface2, BorderRadius: 10, Wrap: giode.WrapNoWrap}, kids...)
}
