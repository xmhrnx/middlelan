//go:build !(darwin && !ios)

package ui

import "github.com/mehran9675/giode"

// titleBarVariant matches the platform: square buttons on the right.
var titleBarVariant = giode.TitleBarWindows
