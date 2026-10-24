package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"unified-inbox/internal/inbox"
)

// App is the bridge between the React UI and the inbox service. Every
// exported method is callable from the frontend via the generated bindings.
type App struct {
	ctx      context.Context
	svc      *inbox.Service
	startErr string

	viewLock   sync.Mutex
	activeRoom string
	focused    bool

	dataDir      string
	settingsLock sync.Mutex
	settings     Settings
}

func NewApp(svc *inbox.Service, dataDir string, settings Settings) *App {
	return &App{svc: svc, dataDir: dataDir, settings: settings}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	applyTheme(ctx, a.GetSettings().Theme)
	a.svc.Emit = func(name string, data any) { runtime.EventsEmit(ctx, name, data) }
	a.setupNotifications()
	go cleanOpenedMedia()
	go func() {
		if err := a.svc.Start(context.Background()); err != nil {
			a.svc.Log.Err(err).Msg("Failed to start inbox")
			a.startErr = err.Error()
			runtime.EventsEmit(ctx, "inbox:error", a.startErr)
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	runtime.CleanupNotifications(ctx)
	a.svc.Stop()
}

func (a *App) setupNotifications() {
	if err := runtime.InitializeNotifications(a.ctx); err != nil {
		a.svc.Log.Warn().Err(err).Msg("Desktop notifications unavailable")
		return
	}
	if !a.GetSettings().NotificationsOff {
		a.requestNotificationPermission()
	}
	// Clicking a notification brings the window forward on that chat.
	runtime.OnNotificationResponse(a.ctx, func(result runtime.NotificationResult) {
		if roomID, ok := result.Response.UserInfo["roomId"].(string); ok {
			runtime.WindowShow(a.ctx)
			runtime.WindowUnminimise(a.ctx)
			runtime.EventsEmit(a.ctx, "inbox:open", roomID)
		}
	})
	a.svc.OnNotify = func(n inbox.Notification) {
		settings := a.GetSettings()
		if settings.NotificationsOff {
			a.svc.Log.Debug().Str("room_id", n.RoomID).Msg("Not notifying: turned off in Settings")
			return
		}
		a.viewLock.Lock()
		looking := a.focused && a.activeRoom == n.RoomID
		a.viewLock.Unlock()
		if looking {
			a.svc.Log.Debug().Str("room_id", n.RoomID).Msg("Not notifying: chat is open and focused")
			return
		}
		body := n.Body
		if settings.HideMessageText {
			body = hiddenMessageText
		}
		if n.Sender != "" {
			body = n.Sender + ": " + body
		}
		err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
			ID:    n.RoomID,
			Title: n.Title,
			Body:  body,
			Data:  map[string]any{"roomId": n.RoomID},
		})
		if err != nil {
			a.svc.Log.Warn().Err(err).Str("room_id", n.RoomID).Msg("Failed to send notification")
		} else {
			// Only the chat is logged, never the message.
			a.svc.Log.Info().Str("room_id", n.RoomID).Msg("Sent notification")
		}
	}
}

// SetView tells the backend which chat is open and whether the window has
// focus, so we don't notify about messages the user is already reading.
func (a *App) SetView(roomID string, focused bool) {
	a.viewLock.Lock()
	a.activeRoom, a.focused = roomID, focused
	a.viewLock.Unlock()
	if focused && roomID != "" {
		_ = runtime.RemoveDeliveredNotification(a.ctx, roomID)
	}
}

func (a *App) StartupError() string { return a.startErr }

func (a *App) Networks() []inbox.LoginFlow { return a.svc.Networks() }

func (a *App) LoginFlows(network string) ([]inbox.LoginFlow, error) {
	return a.svc.LoginFlows(network)
}

func (a *App) StartLogin(network, flowID string) (*inbox.LoginStep, error) {
	return a.svc.StartLogin(network, flowID)
}

func (a *App) SubmitLogin(sessionID string, values map[string]string) (*inbox.LoginStep, error) {
	return a.svc.SubmitLogin(sessionID, values)
}

func (a *App) CancelLogin(sessionID string) { a.svc.CancelLogin(sessionID) }

// BrowserLogin completes a cookie step in the built-in login window.
func (a *App) BrowserLogin(sessionID string) (*inbox.LoginStep, error) {
	return a.svc.BrowserLogin(sessionID)
}

func (a *App) Accounts() ([]inbox.Account, error) { return a.svc.Accounts() }

func (a *App) Logout(network, loginID string) error { return a.svc.Logout(network, loginID) }

func (a *App) Chats() ([]inbox.Chat, error) { return a.svc.Chats(a.ctx) }

func (a *App) Messages(roomID string, beforeTS int64, limit int) ([]inbox.Message, error) {
	return a.svc.Messages(a.ctx, roomID, beforeTS, limit)
}

func (a *App) SendText(roomID, text, replyTo string) error {
	return a.svc.SendText(a.ctx, roomID, text, replyTo)
}

// SendFiles sends local files (from drag and drop) to a chat.
func (a *App) SendFiles(roomID string, paths []string) error {
	var errs []string
	for _, path := range paths {
		if err := a.svc.SendFile(a.ctx, roomID, path); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// PickAndSendFiles opens the native file picker and sends the chosen files.
func (a *App) PickAndSendFiles(roomID string) error {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "Send files"})
	if err != nil || len(paths) == 0 {
		return err
	}
	return a.SendFiles(roomID, paths)
}

func (a *App) React(roomID, eventID, key string) error {
	return a.svc.React(a.ctx, roomID, eventID, key)
}

func (a *App) MarkRead(roomID string) error { return a.svc.MarkRead(a.ctx, roomID) }

// ResolveMedia fetches the real media for a shared Instagram reel, story or
// post that was stored as a thumbnail; the message updates when it arrives.
func (a *App) ResolveMedia(roomID, eventID string) error {
	return a.svc.ResolveMedia(a.ctx, roomID, eventID)
}

// SetChatHidden hides a chat from the list (until a new message arrives) or
// brings it back.
func (a *App) SetChatHidden(roomID string, hidden bool) error {
	return a.svc.SetHidden(a.ctx, roomID, hidden)
}

func (a *App) SearchPeople(query string) ([]inbox.Person, error) {
	return a.svc.SearchPeople(a.ctx, query)
}

func (a *App) OpenChat(network, loginID, userID string) (string, error) {
	return a.svc.OpenChat(a.ctx, network, loginID, userID)
}

// dataDir is ~/Library/Application Support/Pigeonhole on macOS and
// %AppData%\Pigeonhole on Windows.
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "Pigeonhole")
	if err = migrateLegacyDataDir(filepath.Join(base, "UnifiedInbox"), dir); err != nil {
		return "", err
	}
	return dir, os.MkdirAll(filepath.Join(dir, "logs"), 0o700)
}

// migrateLegacyDataDir moves data from before the app was renamed from
// "Unified Inbox" to Pigeonhole, so logins and history carry over. It only
// runs when the new folder doesn't exist yet.
func migrateLegacyDataDir(legacy, dir string) error {
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}
	if err := os.Rename(legacy, dir); err != nil {
		return fmt.Errorf("failed to move data from %s to %s: %w", legacy, dir, err)
	}
	return nil
}

func newLogger(dir string) zerolog.Logger {
	writers := []io.Writer{zerolog.ConsoleWriter{Out: os.Stderr}}
	if f, err := os.OpenFile(filepath.Join(dir, "logs", "inbox.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		writers = append(writers, f)
	}
	return zerolog.New(zerolog.MultiLevelWriter(writers...)).Level(zerolog.DebugLevel).With().Timestamp().Logger()
}
