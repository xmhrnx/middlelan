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
	{screenDashboard, "Overview", "layout-dashboard", true},
	{screenApps, "Apps", "app-window", true},
	{screenConnections, "Connections", "waypoints", false},
	{screenMap, "Map", "globe", true},
	{screenDevices, "Devices", "radar", true},
	{screenTools, "Tools", "wrench", true},
	{screenSettings, "Settings", "settings", false},
}

func (s screen) title() string {
	for _, n := range navItems {
		if n.screen == s {
			return n.label
		}
	}
	return ""
}
