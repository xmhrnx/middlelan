package ui

import (
	"context"
	"fmt"

	"github.com/mehran9675/giode"
	"github.com/xmhrnx/middlelan/internal/nettools"
)

// speedTool measures the internet connection.
type speedTool struct {
	shell   *Shell
	run     *giode.ButtonElement
	runner  toolRunner
	update  nettools.SpeedUpdate
	result  *nettools.SpeedResult
	history []nettools.SpeedResult
	err     string
}

func newSpeedTool(sh *Shell) *speedTool {
	s := &speedTool{shell: sh, runner: toolRunner{shell: sh}}
	s.run = runButton("Start test", s.start, &s.runner)
	return s
}

func (s *speedTool) setTarget(string) {}

func (s *speedTool) start() {
	s.update, s.result, s.err = nettools.SpeedUpdate{}, nil, ""
	s.runner.start(func(ctx context.Context) {
		res, err := nettools.SpeedTest(ctx, func(u nettools.SpeedUpdate) {
			s.runner.post(func() { s.update = u })
		})
		s.runner.post(func() {
			if err != nil {
				if ctx.Err() == nil {
					s.err = err.Error()
				}
				return
			}
			s.result = &res
			s.history = append([]nettools.SpeedResult{res}, s.history...)
			if len(s.history) > 10 {
				s.history = s.history[:10]
			}
		})
	})
}

func (s *speedTool) view(t *toolsScreen, info toolInfo) giode.Element {
	mbps, label, c := 0.0, "Ready", colDown
	switch {
	case s.runner.running:
		mbps, label = s.update.Mbps, s.update.Phase.String()
		if s.update.Phase == nettools.PhaseUpload {
			c = colUp
		}
	case s.result != nil:
		mbps, label = s.result.DownloadMbps, "Download"
	}
	progress := float32(s.update.Progress)
	hero := cardStyled(giode.Styles{Align: giode.AlignCenter, Gap: 16, Padding: giode.UniformInset(24)},
		speedGauge(mbps, label, c),
		giode.Box(giode.Styles{Width: 280}, giode.Progress(progress, giode.Styles{Color: c, Background: colGrid, Height: 4})),
		syncRunLabel(s.run, "Start test", &s.runner),
		giode.Text("Uses Cloudflare's speed test servers. A test transfers about 100 MB.", styleMuted),
	)
	results := []giode.Element{hero}
	if s.err != "" {
		results = append(results, noticeBanner("shield-alert", "The test failed: "+s.err, "theme.danger", nil))
	}
	if r := s.result; r != nil {
		results = append(results, responsiveGrid(150, 4, 12,
			statTile("arrow-down", "Download", fmt.Sprintf("%.1f Mbps", r.DownloadMbps), "", colDown),
			statTile("arrow-up", "Upload", fmt.Sprintf("%.1f Mbps", r.UploadMbps), "", colUp),
			statTile("activity", "Latency", fmt.Sprintf("%.0f ms", r.LatencyMs), "Idle round trip", "theme.success"),
			statTile("signal", "Jitter", fmt.Sprintf("%.1f ms", r.JitterMs), "Latency variation", "theme.warning"),
		))
	}
	if len(s.history) > 1 {
		rows := []giode.Element{}
		for _, h := range s.history {
			rows = append(rows, giode.Row(giode.Styles{Gap: 12, Width: -1, Padding: giode.SymmetricInset(2, 6)},
				giode.Text(h.When.Format("Jan 2 15:04"), giode.Styles{Color: "theme.muted", FontSize: sizeSmall, FlexGrow: 1}),
				ratePair(fmt.Sprintf("%.1f", h.DownloadMbps), fmt.Sprintf("%.1f", h.UploadMbps), 12),
				giode.Text(fmt.Sprintf("%.0f ms", h.LatencyMs), styleMuted),
			))
		}
		results = append(results, card("Earlier tests", nil, giode.Stack(giode.Styles{Width: -1}, withDividers(rows)...)))
	}
	return t.toolFrame(info, nil, results...)
}
