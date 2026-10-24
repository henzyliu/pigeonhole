package main

import (
	goruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// hiddenMessageText replaces the message in alerts when Settings hides it.
const hiddenMessageText = "New message"

func (a *App) requestNotificationPermission() {
	go func() {
		if ok, err := runtime.RequestNotificationAuthorization(a.ctx); err != nil || !ok {
			a.svc.Log.Warn().Err(err).Bool("granted", ok).Msg("Notification permission not granted")
		} else {
			a.svc.Log.Info().Msg("Notification permission granted")
		}
	}()
}

// NotificationPermission is whether the operating system lets Pigeonhole
// show notifications, for Settings.
type NotificationPermission struct {
	// Status is "allowed", "blocked" or "unknown".
	Status string `json:"status"`
	// System names the operating system, for wording ("macOS", "Windows").
	System string `json:"system"`
}

// NotificationPermission checks whether the system allows Pigeonhole's
// notifications. Wails asks macOS; on Windows it always says yes, so the
// app's switch in Windows' settings is read directly (see
// windowsBlocksNotifications).
func (a *App) NotificationPermission() NotificationPermission {
	p := NotificationPermission{Status: "unknown", System: systemName()}
	if blocked, known := windowsBlocksNotifications(); known {
		p.Status = map[bool]string{true: "blocked", false: "allowed"}[blocked]
		return p
	}
	if ok, err := runtime.CheckNotificationAuthorization(a.ctx); err == nil {
		p.Status = map[bool]string{true: "allowed", false: "blocked"}[ok]
	}
	return p
}

// OpenNotificationSettings opens the system's notification settings.
func (a *App) OpenNotificationSettings() error {
	switch goruntime.GOOS {
	case "darwin":
		return openWithDefaultApp("x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=" + macBundleID)
	case "windows":
		return openWithDefaultApp("ms-settings:notifications")
	}
	return nil
}

// macBundleID is the app's identifier on macOS (Wails' default for the name
// in wails.json), so System Settings opens on Pigeonhole's entry.
const macBundleID = "com.wails.pigeonhole"

func systemName() string {
	switch goruntime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return "your system"
}
