// Package settings loads and saves MiddleMan's preferences.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Settings are the user's preferences.
type Settings struct {
	// Theme is "dark", "light" or "system".
	Theme string `json:"theme"`
	// Language is the interface language ("es", "fa", ...); empty
	// follows the system.
	Language string `json:"language,omitempty"`
	// OnlineLookups allows locating remote addresses through geojs.io.
	OnlineLookups bool `json:"onlineLookups"`
	// NotifyNewApps notifies when a program uses the network for the first time.
	NotifyNewApps bool `json:"notifyNewApps"`
	// NotifyNewDevices notifies when an unknown device joins the network.
	NotifyNewDevices bool `json:"notifyNewDevices"`
	// BackgroundScan rescans the local network periodically.
	BackgroundScan bool `json:"backgroundScan"`
	// ScanEveryMinutes is the interval of background scans.
	ScanEveryMinutes int `json:"scanEveryMinutes"`
	// FastSampling samples every second instead of every two.
	FastSampling bool `json:"fastSampling"`
	// CloseToTray keeps monitoring when the window is closed.
	CloseToTray bool `json:"closeToTray"`
	// KeepAliveAndroid keeps monitoring in the background on Android.
	KeepAliveAndroid bool `json:"keepAliveAndroid"`
	// ScheduledSpeedTests runs a speed test every SpeedEveryHours (off by
	// default: each test transfers about 100 MB).
	ScheduledSpeedTests bool `json:"scheduledSpeedTests"`
	SpeedEveryHours     int  `json:"speedEveryHours"`
	// PlanDownMbps and PlanUpMbps are what the internet plan promises,
	// for the ISP report card.
	PlanDownMbps float64 `json:"planDownMbps"`
	PlanUpMbps   float64 `json:"planUpMbps"`
	// ThreatAlerts checks destinations against public blocklists.
	ThreatAlerts bool `json:"threatAlerts"`
	// AdBlock answers queries for ad and tracker domains on the device
	// (Android, while the on-device VPN runs). Off by default.
	AdBlock bool `json:"adBlock"`
	// AdBlockAllow are domains never blocked, with their subdomains.
	AdBlockAllow []string `json:"adBlockAllow,omitempty"`
	// FirewallAsk asks what to do when a program connects for the first
	// time (only while the firewall is on).
	FirewallAsk bool `json:"firewallAsk"`
	// CheckUpdates looks for a newer MiddleMan once a day.
	CheckUpdates bool `json:"checkUpdates"`
	// OnboardingDone hides the welcome card.
	OnboardingDone bool `json:"onboardingDone"`
}

// Defaults returns the settings of a fresh install.
func Defaults() Settings {
	return Settings{
		Theme: "dark", OnlineLookups: true, NotifyNewApps: true, NotifyNewDevices: true,
		BackgroundScan: true, ScanEveryMinutes: 15, CloseToTray: true, KeepAliveAndroid: false,
		SpeedEveryHours: 6, CheckUpdates: true, ThreatAlerts: true,
	}
}

// File is a settings file. Its methods are safe from any goroutine.
type File struct {
	mu   sync.Mutex
	path string
	s    Settings
}

// Open loads settings from dir, using defaults when there's no file yet.
func Open(dir string) (*File, error) {
	f := &File{path: filepath.Join(dir, "settings.json"), s: Defaults()}
	b, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f.s); err != nil {
		return f, err
	}
	if f.s.ScanEveryMinutes <= 0 {
		f.s.ScanEveryMinutes = 15
	}
	if f.s.SpeedEveryHours <= 0 {
		f.s.SpeedEveryHours = 6
	}
	return f, nil
}

// Get returns the current settings.
func (f *File) Get() Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.s
}

// Update changes the settings and saves them.
func (f *File) Update(fn func(*Settings)) error {
	f.mu.Lock()
	fn(&f.s)
	b, err := json.MarshalIndent(f.s, "", "  ")
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}
