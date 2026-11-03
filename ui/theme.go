// Package ui is MiddleMan's interface: the app shell, screens and the
// components they share. It reads data from the internal packages and
// never does network or file work while building a frame.
package ui

import (
	"github.com/xmhrnx/giode"
)

// Palettes: neutral graphite with champagne gold. Download
// traffic is always gold and upload platinum, so the two read the same
// in every chart, list and tile.
var (
	darkPalette = giode.Palette{
		Primary: "#d9b36c", Secondary: "#a3a3a6", Background: "#0b0b0c", Surface: "#151516",
		Text: "#f2f2f0", Border: "#27272a", OnPrimary: "#1a1408", Muted: "#98989d",
		Accent: "#c8c8cc", Success: "#7fc79a", Warning: "#e8975a", Danger: "#e5736b", Info: "#a7b4c2",
	}
	lightPalette = giode.Palette{
		Primary: "#9c7130", Secondary: "#737378", Background: "#f5f5f6", Surface: "#ffffff",
		Text: "#18181b", Border: "#e4e4e7", OnPrimary: "#ffffff", Muted: "#6b6b72",
		Accent: "#5a5a60", Success: "#2f8a55", Warning: "#b8611e", Danger: "#c0392b", Info: "#4f6478",
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
	colSelected                = giode.Mix("theme.text", "theme.surface", 0.09)
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
		BoxShadow: giode.BoxShadow{Y: 6, Blur: 24, Color: "#00000029"},
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
