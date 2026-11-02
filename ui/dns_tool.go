package ui

import (
	"context"
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// dnsTool queries DNS records.
type dnsTool struct {
	shell        *Shell
	name, server *input.Input
	run          *giode.ButtonElement
	runner       toolRunner
	records      []nettools.DNSRecord
	took         time.Duration
	err          string
}

func newDNSTool(sh *Shell) *dnsTool {
	d := &dnsTool{shell: sh, runner: toolRunner{shell: sh}}
	d.name = textInputT("Domain or IP, e.g. example.com", styleField).Keyboard(giode.KeyboardURL)
	d.name.Submit(func(string) { d.start() })
	d.server = textInputT("DNS server (optional), e.g. 1.1.1.1", styleField)
	d.run = runButton("Look up", d.start, &d.runner)
	return d
}

func (d *dnsTool) setTarget(host string) { d.name.SetText(host) }

func (d *dnsTool) start() {
	name := strings.TrimSpace(d.name.Text())
	if name == "" {
		return
	}
	server := strings.TrimSpace(d.server.Text())
	d.records, d.err = nil, ""
	d.runner.start(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		start := time.Now()
		recs, err := nettools.LookupDNS(ctx, name, server)
		took := time.Since(start)
		d.runner.post(func() {
			d.records, d.took = recs, took
			if err != nil {
				d.err = err.Error()
			}
		})
	})
}

func (d *dnsTool) view(t *toolsScreen, info toolInfo) giode.Element {
	form := giode.Stack(giode.Styles{Gap: 12, Width: -1}, formRow(d.name, syncRunLabel(d.run, "Look up", &d.runner)), d.server)
	results := []giode.Element{}
	if d.err != "" {
		results = append(results, noticeBanner("shield-alert", d.err, "theme.warning", nil))
	}
	if len(d.records) > 0 {
		rows := []giode.Element{}
		for _, r := range d.records {
			rows = append(rows, giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap, Padding: giode.SymmetricInset(2, 5)},
				giode.Box(giode.Styles{Width: 64}, pill(r.Type, "theme.primary")),
				giode.Text(r.Value, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontFamily: fontMono, FlexGrow: 1}).Selectable("dns-"+r.Type+r.Value),
			))
		}
		results = append(results, card(tr("Records"), giode.Text("Answered in "+formatMs(d.took), styleMuted), giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...)))
	}
	return t.toolFrame(info, form, results...)
}
