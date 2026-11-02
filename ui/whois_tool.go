package ui

import (
	"context"
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// whoisTool shows registration data for a domain or address.
type whoisTool struct {
	shell  *Shell
	query  *input.Input
	run    *giode.ButtonElement
	runner toolRunner
	result string
	err    string
}

func newWhoisTool(sh *Shell) *whoisTool {
	w := &whoisTool{shell: sh, runner: toolRunner{shell: sh}}
	w.query = textInputT("Domain or IP, e.g. example.com", styleField).Keyboard(giode.KeyboardURL)
	w.query.Submit(func(string) { w.start() })
	w.run = runButton("Look up", w.start, &w.runner)
	return w
}

func (w *whoisTool) setTarget(host string) { w.query.SetText(host) }

func (w *whoisTool) start() {
	q := strings.TrimSpace(w.query.Text())
	if q == "" {
		return
	}
	w.result, w.err = "", ""
	w.runner.start(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, err := nettools.Whois(ctx, q)
		w.runner.post(func() {
			w.result = strings.TrimSpace(out)
			if err != nil {
				w.err = err.Error()
			}
		})
	})
}

func (w *whoisTool) view(t *toolsScreen, info toolInfo) giode.Element {
	results := []giode.Element{}
	if w.err != "" {
		results = append(results, noticeBanner("shield-alert", w.err, "theme.warning", nil))
	}
	if w.runner.running {
		results = append(results, giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter}, giode.Spinner(giode.Styles{Width: 16, Color: "theme.primary"}), giode.Text(tr("Asking the registry…"), styleMuted)))
	}
	if w.result != "" {
		results = append(results, card(tr("Registration"), nil,
			giode.Text(w.result, giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontFamily: fontMono, LineHeight: 18}).Selectable("whois")))
	}
	return t.toolFrame(info, formRow(w.query, syncRunLabel(w.run, "Look up", &w.runner)), results...)
}
