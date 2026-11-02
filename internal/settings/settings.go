// Package settings loads and saves MiddleLAN's preferences.
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
	// StartAtLogin starts MiddleLAN hidden when the user logs in.
	StartAtLogin bool `json:"startAtLogin"`
	// CloseToTray keeps monitoring when the window is closed.
	CloseToTray bool `json:"closeToTray"`
	// KeepAliveAndroid keeps monitoring in the background on Android.
	KeepAliveAndroid bool `json:"keepAliveAndroid"`
	// OnboardingDone hides the welcome card.
	OnboardingDone bool `json:"onboardingDone"`
}

// Defaults returns the settings of a fresh install.
func Defaults() Settings {
	return Settings{
		Theme: "dark", OnlineLookups: true, NotifyNewApps: true, NotifyNewDevices: true,
		BackgroundScan: true, ScanEveryMinutes: 15, CloseToTray: true, KeepAliveAndroid: false,
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
