package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"unified-inbox/internal/inbox"
)

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := loadSettings(dir).Theme; got != "system" {
		t.Fatalf("missing file should default to system, got %q", got)
	}
	if err := (Settings{Theme: "dark"}).save(dir); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(dir).Theme; got != "dark" {
		t.Fatalf("expected the saved theme, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"purple"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(dir).Theme; got != "system" {
		t.Fatalf("unknown theme should fall back to system, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(dir).Theme; got != "system" {
		t.Fatalf("corrupt file should fall back to system, got %q", got)
	}
}

// Settings files from before the notification options keep alerts on with
// message text; the options round-trip once set.
func TestNotificationSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(nil, dir, loadSettings(dir), false)
	if s := app.GetSettings(); s.NotificationsOff || s.HideMessageText || s.Theme != "dark" {
		t.Fatalf("old settings should keep notifications on with text: %+v", s)
	}
	if err := app.SetHideMessageText(true); err != nil {
		t.Fatal(err)
	}
	if err := app.updateSettings(func(s *Settings) { s.NotificationsOff = true }); err != nil {
		t.Fatal(err)
	}
	if s := loadSettings(dir); !s.NotificationsOff || !s.HideMessageText || s.Theme != "dark" {
		t.Fatalf("expected the options to be saved: %+v", s)
	}
}

// Muting one account's alerts is saved per account and can be undone.
func TestAccountNotifications(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(nil, dir, loadSettings(dir), false)
	if err := app.SetAccountNotifications("discord", "123", false); err != nil {
		t.Fatal(err)
	}
	if err := app.SetAccountNotifications("discord", "123", false); err != nil {
		t.Fatal(err)
	}
	s := loadSettings(dir)
	if !s.accountMuted("discord", "123") || s.accountMuted("whatsapp", "123") || len(s.MutedAccounts) != 1 {
		t.Fatalf("expected only Discord muted, once: %+v", s.MutedAccounts)
	}
	if err := app.SetAccountNotifications("discord", "123", true); err != nil {
		t.Fatal(err)
	}
	if s = loadSettings(dir); len(s.MutedAccounts) != 0 {
		t.Fatalf("expected no muted accounts: %+v", s.MutedAccounts)
	}
}

// Settings saved before the welcome and the tabs option existed keep the
// tabs and haven't finished the welcome; existing users still don't see it,
// because they have chats. Turning tabs off is saved.
func TestWelcomeAndTabsSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadSettings(dir)
	if s.SingleChatList || s.WelcomeDone {
		t.Fatalf("old settings should keep tabs and an unfinished welcome: %+v", s)
	}
	svc, err := inbox.New(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(svc, dir, s, false)
	a.ctx = context.Background()
	if !a.ShowWelcome() {
		t.Fatal("no chats and no finished welcome: show it")
	}
	if err = a.FinishWelcome(); err != nil || a.ShowWelcome() {
		t.Fatalf("a finished welcome shouldn't show again (%v)", err)
	}
	if err = a.SetSeparateChats(false); err != nil {
		t.Fatal(err)
	}
	if saved := loadSettings(dir); !saved.SingleChatList || !saved.WelcomeDone {
		t.Fatalf("settings should be saved: %+v", saved)
	}
	if err = a.SetSeparateChats(true); err != nil || loadSettings(dir).SingleChatList {
		t.Fatalf("turning tabs back on should be saved (%v)", err)
	}
}
