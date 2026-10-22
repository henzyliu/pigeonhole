package main

import (
	"context"
	"io"
	"os"
	"path/filepath"

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
}

func NewApp(svc *inbox.Service) *App {
	return &App{svc: svc}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.svc.Emit = func(name string, data any) { runtime.EventsEmit(ctx, name, data) }
	go func() {
		if err := a.svc.Start(context.Background()); err != nil {
			a.svc.Log.Err(err).Msg("Failed to start inbox")
			a.startErr = err.Error()
			runtime.EventsEmit(ctx, "inbox:error", a.startErr)
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.svc.Stop()
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

func (a *App) Accounts() ([]inbox.Account, error) { return a.svc.Accounts() }

func (a *App) Logout(network, loginID string) error { return a.svc.Logout(network, loginID) }

func (a *App) Chats() ([]inbox.Chat, error) { return a.svc.Chats(a.ctx) }

func (a *App) Messages(roomID string, beforeTS int64, limit int) ([]inbox.Message, error) {
	return a.svc.Messages(a.ctx, roomID, beforeTS, limit)
}

func (a *App) SendText(roomID, text, replyTo string) error {
	return a.svc.SendText(a.ctx, roomID, text, replyTo)
}

func (a *App) React(roomID, eventID, key string) error {
	return a.svc.React(a.ctx, roomID, eventID, key)
}

func (a *App) MarkRead(roomID string) error { return a.svc.MarkRead(a.ctx, roomID) }

// dataDir is ~/Library/Application Support/UnifiedInbox on macOS and
// %AppData%\UnifiedInbox on Windows.
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "UnifiedInbox")
	return dir, os.MkdirAll(filepath.Join(dir, "logs"), 0o700)
}

func newLogger(dir string) zerolog.Logger {
	writers := []io.Writer{zerolog.ConsoleWriter{Out: os.Stderr}}
	if f, err := os.OpenFile(filepath.Join(dir, "logs", "inbox.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		writers = append(writers, f)
	}
	return zerolog.New(zerolog.MultiLevelWriter(writers...)).Level(zerolog.DebugLevel).With().Timestamp().Logger()
}
