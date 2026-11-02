//go:build !(darwin && !ios)

package ui

import "github.com/xmhrnx/giode"

// titleBarVariant matches the platform: square buttons on the right.
var titleBarVariant = giode.TitleBarWindows
