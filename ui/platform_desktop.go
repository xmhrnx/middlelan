//go:build !android && !ios

package ui

// canElevate reports whether the app can restart with administrator rights.
func canElevate() bool { return true }

// isMobile reports whether this is a phone or tablet build.
const isMobile = false
