// Package assets embeds MiddleLAN's icons, fonts and map data.
package assets

import (
	"embed"
	_ "embed"
)

// Icon is the app icon (1024×1024 PNG).
//
//go:embed icon.png
var Icon []byte

// Tray is the colored tray icon; TrayTemplate the black one macOS tints.
var (
	//go:embed tray.png
	Tray []byte
	//go:embed tray-template.png
	TrayTemplate []byte
)

// World is the land mask of the connection map: a little-endian header
// (columns, rows uint16; north and south edge latitudes ×10, int16)
// followed by one bit per cell, row by row from the north.
//
//go:embed world.bin
var World []byte

// Icons holds the UI icons, as SVG files under "icons".
//
//go:embed icons/*.svg
var Icons embed.FS

// Fonts holds the Inter typeface, under "fonts".
//
//go:embed fonts/*.otf
var Fonts embed.FS
