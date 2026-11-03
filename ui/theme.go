// Package ui is MiddleMan's interface: the app shell, screens and the
// components they share. It reads data from the internal packages and
// never does network or file work while building a frame.
package ui

import (
	"github.com/xmhrnx/giode"
)

// Palettes. Download traffic is always cyan and upload violet, so the two
// read the same in every chart, list and tile.
var (
	darkPalette = giode.Palette{
		Primary: "#22d3ee", Secondary: "#94a3b8", Background: "#080b16", Surface: "#10142a",
		Text: "#e8ebf7", Border: "#1f2547", OnPrimary: "#031018", Muted: "#8188ad",
		Accent: "#a78bfa", Success: "#34d399", Warning: "#fbbf24", Danger: "#f87171", Info: "#60a5fa",
	}
	lightPalette = giode.Palette{
		Primary: "#0891b2", Secondary: "#64748b", Background: "#f3f5fb", Surface: "#ffffff",
		Text: "#0e1225", Border: "#e2e6f2", OnPrimary: "#ffffff", Muted: "#5c6386",
		Accent: "#7c3aed", Success: "#059669", Warning: "#d97706", Danger: "#dc2626", Info: "#2563eb",
	}
)

// Theme colors beyond the palette, mixed from tokens so they follow the
// light and dark modes.
var (
	colDown        giode.Color = "theme.primary"
	colUp          giode.Color = "theme.accent"
	colSurface2                = giode.Mix("theme.text", "theme.surface", 0.04)
	colSurface3                = giode.Mix("theme.text", "theme.surface", 0.08)
	colHover                   = giode.Mix("theme.text", "theme.surface", 0.06)
	colSelected                = giode.Mix("theme.primary", "theme.surface", 0.14)
	colDownSoft                = giode.Mix("theme.primary", "theme.surface", 0.16)
	colUpSoft                  = giode.Mix("theme.accent", "theme.surface", 0.16)
	colSuccessSoft             = giode.Mix("theme.success", "theme.surface", 0.16)
	colWarningSoft             = giode.Mix("theme.warning", "theme.surface", 0.16)
	colDangerSoft              = giode.Mix("theme.danger", "theme.surface", 0.16)
	colGrid                    = giode.Mix("theme.text", "theme.surface", 0.07)
	colSidebar                 = giode.Mix("theme.text", "theme.background", 0.025)
	colMapLand                 = giode.Mix("theme.text", "theme.surface", 0.16)
)

// Type scale.
const (
	fontMono = "Go Mono"

	sizeDisplay = 30
	sizeTitle   = 22
	sizeHeading = 16
	sizeBody    = 14
	sizeSmall   = 12.5
	sizeTiny    = 11
)

// Shared styles.
var (
	styleCard = giode.Styles{
		FlexDirection: giode.FlexDirectionColumn, Padding: giode.UniformInset(18), Gap: 12,
		Background: "theme.surface", BorderRadius: 16, BorderWidth: 1, BorderColor: "theme.border",
		BoxShadow: giode.BoxShadow{Y: 6, Blur: 24, Color: "#0000001f"},
	}
	styleTitle   = giode.Styles{Color: "theme.text", FontSize: sizeTitle, FontWeight: giode.FontWeightBold}
	styleHeading = giode.Styles{Color: "theme.text", FontSize: sizeHeading, FontWeight: giode.FontWeightSemiBold}
	styleBody    = giode.Styles{Color: "theme.text", FontSize: sizeBody}
	styleMuted   = giode.Styles{Color: "theme.muted", FontSize: sizeSmall}
	styleLabel   = giode.Styles{Color: "theme.muted", FontSize: sizeTiny, FontWeight: giode.FontWeightSemiBold, TextTransform: giode.TextTransformUppercase}
	styleMono    = giode.Styles{Color: "theme.text", FontSize: sizeSmall, FontFamily: fontMono}
	styleField   = giode.Styles{Background: colSurface2, BorderColor: "theme.border", BorderRadius: 10, Color: "theme.text"}
	stylePrimary = giode.Styles{Background: "theme.primary", Color: "theme.onprimary", BorderRadius: 10, FontWeight: giode.FontWeightSemiBold, Padding: giode.SymmetricInset(18, 10)}
	styleGhost   = giode.Styles{Background: "theme.text", Color: "theme.text", BorderRadius: 10, Padding: giode.SymmetricInset(14, 10)}
	styleOutline = giode.Styles{BorderColor: "theme.border", Color: "theme.text", BorderRadius: 10, Padding: giode.SymmetricInset(14, 9)}
)

// applyTheme switches between the dark and light palettes.
func applyTheme(mode string) {
	giode.SetPalette(giode.ModeDark, darkPalette)
	giode.SetPalette(giode.ModeLight, lightPalette)
	if mode == "light" {
		giode.SetMode(giode.ModeLight)
	} else {
		giode.SetMode(giode.ModeDark)
	}
}
