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
