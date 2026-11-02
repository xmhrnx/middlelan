//go:build !android

package ui

// requestLocationPermission is needed on Android only; desktops ask in
// the system settings.
func requestLocationPermission() error { return nil }
