//go:build windows

package autostart

import (
	"golang.org/x/sys/windows/registry"
)

const supported = true

// The per-user Run key. Windows lists it under Settings → Apps → Startup
// (and Task Manager's Startup tab), where it can be switched off too.
const (
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "Pigeonhole"
)

func enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	return err == nil
}

func enable(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(valueName, `"`+exe+`" `+BackgroundFlag)
}

func disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if err = k.DeleteValue(valueName); err == registry.ErrNotExist {
		return nil
	}
	return err
}
