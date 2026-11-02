package main

import (
	"log"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/middlelan/internal/settings"
)

// releaseKeepAlive ends the Android foreground service, when one runs.
var releaseKeepAlive func()

// applyKeepAlive starts or stops background monitoring on Android (a
// foreground service with an ongoing notification). Desktops keep running
// in the tray and need nothing.
func applyKeepAlive(app *giode.App, s settings.Settings) {
	if !isAndroid {
		return
	}
	switch {
	case s.KeepAliveAndroid && releaseKeepAlive == nil:
		release, err := app.KeepAlive("Monitoring your network")
		if err != nil {
			log.Printf("keep alive: %v", err)
			return
		}
		releaseKeepAlive = release
	case !s.KeepAliveAndroid && releaseKeepAlive != nil:
		releaseKeepAlive()
		releaseKeepAlive = nil
	}
}
