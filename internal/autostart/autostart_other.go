//go:build !(linux && !android) && !(darwin && !ios) && !windows

package autostart

import "errors"

const supported = false

var errUnsupported = errors.New("starting at login isn't supported on this platform")

// Enable registers the running executable to start at login.
func Enable() error { return errUnsupported }

// Disable removes the login item.
func Disable() error { return errUnsupported }
