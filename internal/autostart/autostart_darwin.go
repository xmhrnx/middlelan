//go:build darwin && !ios

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const supported = true

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.middlelan.app.plist"), nil
}

// Enable registers the running executable to start at login.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.middlelan.app</string>
<key>ProgramArguments</key><array><string>%s</string><string>%s</string></array>
<key>RunAtLoad</key><true/>
<key>ProcessType</key><string>Interactive</string>
</dict></plist>
`, exe, HiddenFlag)
	return os.WriteFile(path, []byte(plist), 0o644)
}

// Disable removes the login item.
func Disable() error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
