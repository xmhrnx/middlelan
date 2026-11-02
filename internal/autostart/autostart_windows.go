//go:build windows

package autostart

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const supported = true

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// Enable registers the running executable to start at login.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("MiddleLAN", `"`+exe+`" `+HiddenFlag)
}

// Disable removes the login item.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue("MiddleLAN"); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
