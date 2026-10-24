package main

import (
	"context"
	goruntime "runtime"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"unified-inbox/internal/autostart"
	"unified-inbox/internal/tray"
)

// Pigeonhole keeps running when its window is closed (Wails hides it
// instead), so it stays connected and notifying. The menu bar / tray icon
// reopens or quits it, and it can start at login, hidden.

func (a *App) startBackground() {
	tray.Start(tray.Handlers{
		Open:    a.showWindow,
		Quit:    func() { runtime.Quit(a.ctx) },
		AppIcon: appIconPNG,
	})
	// On Windows, Wails starts the window hidden (see main.go). On macOS the
	// app hides itself instead, so clicking the Dock icon brings it back.
	if a.background && goruntime.GOOS == "darwin" {
		tray.HideApp()
	}
	go func() {
		if err := autostart.Refresh(); err != nil {
			a.svc.Log.Warn().Err(err).Msg("Failed to update the start-at-login entry")
		}
	}()
}

// showWindow brings the window back, e.g. from the menu bar or a second
// launch of the app.
func (a *App) showWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	// Windows drops the taskbar badge while the window is hidden; put it
	// back once the taskbar button exists again.
	time.AfterFunc(300*time.Millisecond, tray.RefreshTaskbar)
}

const unreadUpdateDelay = 300 * time.Millisecond

func (a *App) scheduleUnreadUpdate() {
	a.unreadLock.Lock()
	defer a.unreadLock.Unlock()
	if a.unreadTimer != nil {
		return
	}
	a.unreadTimer = time.AfterFunc(unreadUpdateDelay, func() {
		a.unreadLock.Lock()
		a.unreadTimer = nil
		a.unreadLock.Unlock()
		n, err := a.svc.UnreadChats(context.Background())
		if err != nil {
			a.svc.Log.Warn().Err(err).Msg("Failed to count unread chats")
			return
		}
		tray.SetUnread(n)
	})
}

// StartAtLogin is the state of the start-at-login switch in Settings.
type StartAtLogin struct {
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
	// System names the operating system, for wording ("macOS", "Windows").
	System string `json:"system"`
}

// StartAtLogin reports whether Pigeonhole starts when you log in.
func (a *App) StartAtLogin() StartAtLogin {
	return StartAtLogin{Supported: autostart.Supported(), Enabled: autostart.Enabled(), System: systemName()}
}

// SetStartAtLogin turns starting at login on or off.
func (a *App) SetStartAtLogin(on bool) error {
	if err := autostart.Set(on); err != nil {
		a.svc.Log.Warn().Err(err).Bool("on", on).Msg("Failed to change start at login")
		return err
	}
	a.svc.Log.Info().Bool("on", on).Msg("Changed start at login")
	return nil
}

// WindowHidden is called by the UI when the window is hidden (closed or
// minimised). The first time, it says Pigeonhole is still running, since
// closing the window used to quit it.
func (a *App) WindowHidden() {
	settings := a.GetSettings()
	if settings.BackgroundNoticeShown || settings.NotificationsOff {
		return
	}
	where := "menu bar"
	if goruntime.GOOS == "windows" {
		where = "notification area by the clock"
	}
	err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID:    "pigeonhole-background",
		Title: "Pigeonhole is still running",
		Body:  "You'll keep getting messages. Open or quit it from its icon in the " + where + ".",
	})
	if err != nil {
		a.svc.Log.Warn().Err(err).Msg("Failed to show the still-running notice")
		return
	}
	_ = a.updateSettings(func(s *Settings) { s.BackgroundNoticeShown = true })
}
