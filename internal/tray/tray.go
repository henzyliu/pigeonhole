// Package tray puts Pigeonhole in the macOS menu bar or the Windows
// notification area, so it can keep running (and notifying) with its window
// closed. The icon shows the unread count, and its menu reopens or quits
// the app.
package tray

import "sync"

// Handlers are what the icon's menu does. They're called on their own
// goroutine, so they may block.
type Handlers struct {
	Open func()
	Quit func()
	// AppIcon is the app icon as a PNG, drawn into the Windows tray icon
	// (with a red dot when something's unread).
	AppIcon []byte
}

var (
	handlersLock sync.Mutex
	handlers     Handlers
	appIconPNG   []byte
)

// Start shows the icon.
func Start(h Handlers) {
	handlersLock.Lock()
	handlers = h
	handlersLock.Unlock()
	appIconPNG = h.AppIcon
	start()
}

// Stop removes the icon (Windows leaves a stale one otherwise).
func Stop() { stop() }

// SetUnread shows how many chats have unread messages (0 shows none).
func SetUnread(n int) { setUnread(n) }

// RefreshTaskbar puts the unread badge back on the Windows taskbar button,
// which Windows drops while the window is hidden. Call it after showing the
// window.
func RefreshTaskbar() { refreshTaskbar() }

// HideApp hides the app as if its window had been closed. On macOS that's
// how Pigeonhole starts at login: hidden, but still in the Dock and menu
// bar, so clicking either brings it back.
func HideApp() { hideApp() }

func openApp() {
	handlersLock.Lock()
	h := handlers.Open
	handlersLock.Unlock()
	if h != nil {
		go h()
	}
}

func quitApp() {
	handlersLock.Lock()
	h := handlers.Quit
	handlersLock.Unlock()
	if h != nil {
		go h()
	}
}
