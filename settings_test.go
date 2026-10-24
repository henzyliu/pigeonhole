package main

import (
	"os"
	"path/filepath"
	"testing"
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
