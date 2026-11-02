package ui

import (
	"context"
	"strings"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/input"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// tlsTool inspects a server's TLS certificate.
type tlsTool struct {
	shell  *Shell
	host   *input.Input
	run    *giode.ButtonElement
	runner toolRunner
	cert   *nettools.CertInfo
	err    string
}

func newTLSTool(sh *Shell) *tlsTool {
	t := &tlsTool{shell: sh, runner: toolRunner{shell: sh}}
	t.host = textInputT("Host, e.g. example.com or 10.0.0.5:8443", styleField).Keyboard(giode.KeyboardURL)
	t.host.Submit(func(string) { t.start() })
	t.run = runButton("Inspect", t.start, &t.runner)
	return t
}

func (t *tlsTool) setTarget(host string) { t.host.SetText(host) }

func (t *tlsTool) start() {
	h := strings.TrimSpace(t.host.Text())
	h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	h = strings.SplitN(h, "/", 2)[0]
	if h == "" {
		return
	}
	t.cert, t.err = nil, ""
	t.runner.start(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		c, err := nettools.InspectTLS(ctx, h)
		t.runner.post(func() {
			if err != nil {
				t.err = err.Error()
				return
			}
			t.cert = &c
		})
	})
}

func (t *tlsTool) view(ts *toolsScreen, info toolInfo) giode.Element {
	results := []giode.Element{}
	if t.err != "" {
		results = append(results, noticeBanner("shield-alert", t.err, "theme.danger", nil))
	}
	if c := t.cert; c != nil {
		status, tone, icon := tr("Valid certificate"), giode.Color("theme.success"), "shield-check"
		switch {
		case !c.Valid:
			status, tone, icon = tr("Not trusted: {error}", "error", c.VerifyErr), "theme.danger", "shield-alert"
		case c.DaysLeft < 14:
			status, tone = tr("Expires soon"), "theme.warning"
		}
		fact := func(label, value string) giode.Element {
			return giode.Row(giode.Styles{Gap: 12, Width: -1, Padding: giode.SymmetricInset(0, 4), Wrap: giode.WrapNoWrap},
				giode.Text(label, giode.Styles{Color: "theme.muted", FontSize: sizeSmall, Width: 120}),
				giode.Text(nonEmpty(value, "—"), giode.Styles{Color: "theme.text", FontSize: sizeSmall, FlexGrow: 1}))
		}
		results = append(results,
			noticeBanner(icon, status, tone, nil),
			responsiveGrid(150, 3, 12,
				statTile("clock", tr("Days left"), itoa(c.DaysLeft), tr("Until {date}", "date", c.NotAfter.Format(time.DateOnly)), tone),
				statTile("lock", tr("Protocol"), c.TLSVersion, c.Cipher, "theme.primary"),
				statTile("globe", tr("Names"), itoa(len(c.DNSNames)), tr("Covered host names"), "theme.accent"),
			),
			card(tr("Certificate"), nil,
				fact(tr("Subject"), c.Subject), fact(tr("Issuer"), c.Issuer),
				fact(tr("Valid from"), c.NotBefore.Format(time.DateOnly)), fact(tr("Valid until"), c.NotAfter.Format(time.DateOnly)),
				fact(tr("Names"), strings.Join(c.DNSNames, ", ")), fact(tr("Chain"), strings.Join(c.Chain, "  ›  "))),
		)
	}
	return ts.toolFrame(info, formRow(t.host, syncRunLabel(t.run, "Inspect", &t.runner)), results...)
}
