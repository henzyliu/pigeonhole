//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// windowsBlocksNotifications reads Windows' notification switches: the
// global one, and the app's own, which Windows keys by the app ID Wails
// sends toasts with (the executable's file name).
func windowsBlocksNotifications() (blocked, known bool) {
	if off, ok := registryDWORD(`Software\Microsoft\Windows\CurrentVersion\PushNotifications`, "ToastEnabled"); ok && off == 0 {
		return true, true
	}
	exe, err := os.Executable()
	if err != nil {
		return false, false
	}
	key := `Software\Microsoft\Windows\CurrentVersion\Notifications\Settings\` + filepath.Base(exe)
	if enabled, ok := registryDWORD(key, "Enabled"); ok && enabled == 0 {
		return true, true
	}
	// Neither switch is set to off, which is Windows' default.
	return false, true
}

func registryDWORD(path, name string) (uint64, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	return v, err == nil
}
