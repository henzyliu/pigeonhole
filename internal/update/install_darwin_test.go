//go:build darwin

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fakeApp makes a minimal app bundle whose executable prints marker.
func fakeApp(t *testing.T, path, marker string) {
	t.Helper()
	bin := filepath.Join(path, "Contents", "MacOS")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "Pigeonhole"), []byte("#!/bin/sh\necho "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Staging copies the app out of a disk image next to the running one, and
// the swap replaces it only after the running process has exited.
func TestStageAndSwap(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	fakeApp(t, filepath.Join(src, "Pigeonhole.app"), "new")
	dmg := filepath.Join(dir, "update.dmg")
	if out, err := exec.Command("hdiutil", "create", "-volname", "Pigeonhole", "-srcfolder", src, "-size", "10m",
		"-ov", "-format", "UDZO", dmg).CombinedOutput(); err != nil {
		t.Fatalf("hdiutil create: %s", out)
	}
	apps := filepath.Join(dir, "Applications")
	bundle := filepath.Join(apps, "Pigeonhole.app")
	fakeApp(t, bundle, "old")

	staged, err := stage(dmg, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(staged) != apps {
		t.Fatalf("staged at %s, want beside the app", staged)
	}

	// A stand-in for the running app, which quits shortly.
	running := exec.Command("sleep", "1")
	if err = running.Start(); err != nil {
		t.Fatal(err)
	}
	go running.Wait()
	if err = startSwap(running.Process.Pid, bundle, staged, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if out, _ := exec.Command(filepath.Join(bundle, "Contents", "MacOS", "Pigeonhole")).Output(); string(out) != "old\n" {
		t.Fatalf("swapped before the app quit: %q", out)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		out, _ := exec.Command(filepath.Join(bundle, "Contents", "MacOS", "Pigeonhole")).Output()
		if string(out) == "new\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not swapped after the app quit: %q", out)
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	left, _ := os.ReadDir(apps)
	if len(left) != 1 || left[0].Name() != "Pigeonhole.app" {
		t.Fatalf("leftovers beside the app: %v", left)
	}
}
