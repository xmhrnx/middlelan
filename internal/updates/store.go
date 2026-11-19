package updates

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

// ErrStoreBuild means a store installed this copy and updates it: the
// Microsoft Store (an MSIX package) or Google Play. Those stores don't
// want apps pointing users at downloads elsewhere.
var ErrStoreBuild = errors.New("updates for this copy come from its store")

// StoreManaged reports whether a store updates this copy: every Android
// build (Google Play, or the user's own sideloading) and a Windows copy
// installed from an MSIX package.
func StoreManaged() bool {
	exe, _ := os.Executable()
	return storeManaged(runtime.GOOS, exe)
}

func storeManaged(goos, exe string) bool {
	switch goos {
	case "android", "ios":
		return true
	case "windows":
		return strings.Contains(strings.ToLower(exe), `\windowsapps\`)
	}
	return false
}
