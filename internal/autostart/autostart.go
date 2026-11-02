// Package autostart registers MiddleLAN to start, hidden, when the user
// logs in: a .desktop file on Linux, a LaunchAgent on macOS and the Run
// registry key on Windows.
package autostart

// HiddenFlag is the argument the app is started with at login.
const HiddenFlag = "--hidden"

// Supported reports whether starting at login can be set up here.
func Supported() bool { return supported }
