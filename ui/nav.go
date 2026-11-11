package ui

// screen identifies a top-level page.
type screen int

const (
	screenDashboard screen = iota
	screenApps
	screenConnections
	screenMap
	screenDevices
	screenTools
	screenSettings
	screenHistory
	screenSecurity
	// screenAppDetail shows one app; it isn't in the navigation.
	screenAppDetail
)

// navItem describes a page in the navigation.
type navItem struct {
	screen screen
	label  string
	icon   string
	// compact pages appear in the phone's bottom bar.
	compact bool
}

var navItems = []navItem{
	{screenDashboard, msg("Overview"), "layout-dashboard", true},
	{screenApps, msg("Apps"), "app-window", true},
	{screenConnections, msg("Connections"), "waypoints", false},
	{screenMap, msg("Map"), "globe", false},
	{screenHistory, msg("History"), "clock-arrow-up", false},
	{screenDevices, msg("Devices"), "radar", true},
	{screenSecurity, msg("Security"), "shield-check", true},
	{screenTools, msg("Tools"), "wrench", true},
	{screenSettings, msg("Settings"), "settings", false},
}

func (s screen) title() string {
	for _, n := range navItems {
		if n.screen == s {
			return tr(n.label)
		}
	}
	return ""
}
