//go:build android

package main

import (
	"sync"
	_ "unsafe" // go:linkname
)

// Gio (v0.10) answers the first GioView it creates through a rendezvous
// whose "window ready" channel only newWindow receives from, once. When
// Android re-creates the activity in the same process (a configuration
// change, even during launch; or the VPN or the background monitoring
// service keeping the process after the activity was destroyed), the
// new GioView's onCreateView blocks the Android main thread forever
// sending on that channel: the app stays on its splash screen. Gio
// reuses the existing window for the new view once the send goes
// through.
//
// So the channel is swapped for a buffered one before any window
// exists: newWindow still takes the first send, later sends never
// block, and once the first frame is drawn a goroutine drains them.

// gioRendezvous mirrors gioui.org/app.windowRendezvous; only the third
// field is used, the first two are channels and so pointer-sized.
type gioRendezvous struct {
	in, out uintptr
	windows chan struct{}
}

//go:linkname gioMainWindow gioui.org/app.mainWindow
var gioMainWindow *gioRendezvous

func init() {
	if mw := gioMainWindow; mw != nil && mw.windows != nil {
		mw.windows = make(chan struct{}, 1024)
	}
}

var reattachOnce sync.Once

// allowViewReattach drains the sends once the first window exists: the
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
