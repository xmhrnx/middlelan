//go:build android

package main

import (
	"sync"
	_ "unsafe" // go:linkname
)

// Gio (v0.10) answers the first GioView it creates through a rendezvous
// whose "window ready" channel only newWindow receives from, once. When
// Android destroys the activity but the process lives on (the VPN or the
// background monitoring service keeps it), reopening the app creates a
// new GioView, and Gio's onCreateView blocks the Android main thread
// forever sending on that channel: the app stays on its splash screen.
// Gio reuses the existing window for the new view once the send goes
// through, so after the first window is up something has to take the
// sends. This links to that rendezvous and drains its channel.

// gioRendezvous mirrors gioui.org/app.windowRendezvous; only the third
// field is used, the first two are channels and so pointer-sized.
type gioRendezvous struct {
	in, out uintptr
	windows chan struct{}
}

//go:linkname gioMainWindow gioui.org/app.mainWindow
var gioMainWindow *gioRendezvous

var reattachOnce sync.Once

// allowViewReattach starts draining once the first window exists: the
// first frame being drawn means newWindow already took its receive.
func allowViewReattach() {
	reattachOnce.Do(func() {
		mw := gioMainWindow
		if mw == nil || mw.windows == nil {
			return
		}
		go func() {
			for range mw.windows {
			}
		}()
	})
}
