//go:build darwin

package autostart

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The login item is a valid LaunchAgent that starts this executable in the
// background, and turning it off removes it.
func TestLoginItem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if Enabled() {
		t.Fatal("shouldn't start enabled")
	}
	if err := Set(true); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("expected it to be enabled")
	}
	data, err := os.ReadFile(plistPath())
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := executable()
	for _, want := range []string{"<string>" + exe + "</string>", "<string>" + BackgroundFlag + "</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("plist missing %q", want)
		}
	}
	if out, err := exec.Command("plutil", "-lint", plistPath()).CombinedOutput(); err != nil {
		t.Fatalf("invalid plist: %s", out)
	}
	if err = Refresh(); err != nil {
		t.Fatal(err)
	}
	if err = Set(false); err != nil || Enabled() {
		t.Fatalf("expected it to be removed: %v", err)
	}
	if err = Set(false); err != nil {
		t.Fatalf("turning off twice should be fine: %v", err)
	}
}
