//go:build !android

package ui

// pushWidget updates the Android home screen widget; desktops have none.
func pushWidget(down, up, today, top string) error { return nil }
