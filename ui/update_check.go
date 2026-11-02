package ui

import (
	"context"
	"errors"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/updates"
)

// updateState is what the last update check found.
type updateState struct {
	checking bool
	release  *updates.Release
	checked  time.Time
	err      string
	progress float32 // download progress while installing, 0..1
}

// checkUpdates looks for a newer version in the background. manual
// checks report "up to date" too; automatic ones only speak up when
// there's something new.
func (sh *Shell) checkUpdates(manual bool) {
	if sh.svc.Updates == nil || sh.update.checking {
		return
	}
	sh.update.checking = true
	go func() {
		ctx, cancel := context.WithTimeout(sh.ctx, 30*time.Second)
		defer cancel()
		rel, err := sh.svc.Updates.Check(ctx)
		sh.svc.App.Post(func() {
			sh.update.checking, sh.update.checked, sh.update.release, sh.update.err = false, time.Now(), rel, ""
			switch {
			case errors.Is(err, updates.ErrDevBuild):
				if manual {
					sh.Toast(giode.ToastInfo, tr("This is a development build; updates come with released versions."))
				}
			case err != nil:
				sh.update.err = err.Error()
				if manual {
					sh.Toast(giode.ToastWarning, tr("Couldn't check for updates: {error}", "error", err.Error()))
				}
			case rel != nil:
				sh.Toast(giode.ToastInfo, tr("MiddleLAN {version} is available. See Settings → About.", "version", rel.Version))
				if !manual {
					notify(sh.svc.App, tr("MiddleLAN {version} is available", "version", rel.Version), tr("Open MiddleLAN to update."))
				}
			case manual:
				sh.Toast(giode.ToastSuccess, tr("MiddleLAN is up to date."))
			}
		})
	}()
}

// installUpdate installs an update, or opens its release page when it
// can't be installed in place.
func (sh *Shell) installUpdate() {
	rel := sh.update.release
	if rel == nil {
		return
	}
	go func() {
		if !rel.Installable {
			if err := sh.svc.App.OpenURL(sh.ctx, rel.URL); err != nil {
				sh.Toast(giode.ToastError, err.Error())
			}
			return
		}
		err := sh.svc.Updates.Install(sh.ctx, rel, func(done, total int64) {
			if total > 0 {
				sh.svc.App.Post(func() { sh.update.progress = float32(done) / float32(total) })
			}
		})
		// Install restarts the app on success, so reaching here is a failure.
		sh.Toast(giode.ToastError, tr("Update failed: {error}", "error", err.Error()))
	}()
}

// runUpdateChecks checks once a day while enabled.
func (sh *Shell) runUpdateChecks(ctx context.Context) {
	wait := time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 24 * time.Hour
		if sh.svc.Settings.Get().CheckUpdates {
			sh.svc.App.Post(func() { sh.checkUpdates(false) })
		}
	}
}

// notify shows a system notification, falling back to nothing (the
// toast is the in-app channel).
func notify(app *giode.App, title, body string) {
	_ = app.Notify(title, body) // not every desktop has a notification service
}
