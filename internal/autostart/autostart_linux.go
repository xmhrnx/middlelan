//go:build linux && !android

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const supported = true

func desktopFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "middlelan.desktop"), nil
}

// Enable registers the running executable to start at login.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := desktopFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=MiddleLAN\nComment=Network monitor\nExec=%q %s\nX-GNOME-Autostart-enabled=true\nNoDisplay=true\n", exe, HiddenFlag)
	return os.WriteFile(path, []byte(entry), 0o644)
}

// Disable removes the login item.
func Disable() error {
	path, err := desktopFile()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
