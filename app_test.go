package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyDataDir(t *testing.T) {
	base := t.TempDir()
	legacy, dir := filepath.Join(base, "UnifiedInbox"), filepath.Join(base, "Pigeonhole")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "messenger.db"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyDataDir(legacy, dir); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "messenger.db")); err != nil || string(data) != "data" {
		t.Fatalf("expected data to move, got %q, %v", data, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("expected legacy folder to be gone")
	}

	// A second run, or a fresh install with no legacy folder, is a no-op.
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyDataDir(legacy, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy folder should be left alone once the new one exists")
	}
}
