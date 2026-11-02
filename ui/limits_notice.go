package ui

import (
	"github.com/mehran9675/giode"
	"github.com/xmhrnx/middlelan/internal/monitor"
)

// limitsNotice explains what the platform hides and offers the fix:
// administrator rights on desktops, Usage Access on Android.
type limitsNotice struct {
	shell     *Shell
	elevate   *giode.ButtonElement
	usage     *giode.ButtonElement
	dismissed string
	dismiss   *giode.ButtonElement
}

func newLimitsNotice(sh *Shell) *limitsNotice {
	n := &limitsNotice{shell: sh}
	small := giode.Styles{Padding: giode.SymmetricInset(12, 6), BorderRadius: 8, FontSize: sizeSmall}
	n.elevate = giode.Button("Run as administrator", stylePrimary.With(small)).OnClick(func() {
		go func() {
			if err := sh.svc.App.RestartElevated(); err != nil {
				sh.Toast(giode.ToastWarning, "Administrator rights weren't granted: "+err.Error())
			}
		}()
	})
	n.usage = giode.Button("Grant Usage Access", stylePrimary.With(small)).OnClick(func() {
		if err := monitor.OpenUsageAccessSettings(); err != nil {
			sh.Toast(giode.ToastError, err.Error())
		}
	})
	n.dismiss = giode.Button("Hide", styleGhost.With(small)).Variant(giode.VariantGhost).OnClick(func() {
		n.dismissed = sh.svc.Collector.Snapshot().Limits
	})
	return n
}

func (n *limitsNotice) view(s *monitor.Snapshot) giode.Element {
	if s.Err != "" {
		return noticeBanner("shield-alert", "Monitoring error: "+s.Err, "theme.danger", nil)
	}
	if s.Limits == "" || s.Limits == n.dismissed {
		return nil
	}
	var action giode.Element = n.dismiss
	switch {
	case !monitor.UsageAccess():
		action = n.usage
	case !giode.IsElevated() && canElevate():
		action = giode.Row(giode.Styles{Gap: 8, Wrap: giode.WrapNoWrap}, n.dismiss, n.elevate)
	}
	return noticeBanner("info", s.Limits, "theme.info", action)
}
