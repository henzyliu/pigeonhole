//go:build darwin

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// On macOS the update is the release's disk image. The new app is copied
// next to the running one, then a small detached script waits for this
// process to quit, swaps the two (putting the old one back if that fails)
// and opens the new one. Files the app downloads itself aren't quarantined,
// so the new version opens without Gatekeeper's first-launch prompt.

// currentBundle is the .app this process is running from.
func currentBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	// .../Pigeonhole.app/Contents/MacOS/Pigeonhole
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") {
		return "", fmt.Errorf("%w: not running from an app bundle", ErrCantInstall)
	}
	return bundle, nil
}

func canInstall() error {
	bundle, err := currentBundle()
	if err != nil {
		return err
	}
	return checkWritable(filepath.Dir(bundle))
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".pigeonhole-write-test-")
	if err != nil {
		return fmt.Errorf("%w: can't write to %s", ErrCantInstall, dir)
	}
	f.Close()
	return os.Remove(f.Name())
}

func install(file string) error {
	bundle, err := currentBundle()
	if err != nil {
		return err
	}
	staged, err := stage(file, bundle)
	if err != nil {
		return err
	}
	return swapAfterExit(os.Getpid(), bundle, staged)
}

// stage copies the app out of the disk image to sit beside bundle (on the
// same volume, so the swap is a rename), and returns its path.
func stage(dmg, bundle string) (string, error) {
	if err := checkWritable(filepath.Dir(bundle)); err != nil {
		return "", err
	}
	mount, err := os.MkdirTemp("", "pigeonhole-update-")
	if err != nil {
		return "", err
	}
	defer os.Remove(mount)
	out, err := exec.Command("hdiutil", "attach", dmg, "-nobrowse", "-readonly", "-noverify", "-noautoopen",
		"-mountpoint", mount).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("couldn't open the update: %s", strings.TrimSpace(string(out)))
	}
	defer exec.Command("hdiutil", "detach", mount, "-quiet", "-force").Run()

	apps, _ := filepath.Glob(filepath.Join(mount, "*.app"))
	if len(apps) != 1 {
		return "", errors.New("the update doesn't contain the app")
	}
	staged := filepath.Join(filepath.Dir(bundle), ".Pigeonhole-update.app")
	_ = os.RemoveAll(staged)
	if out, err = exec.Command("ditto", apps[0], staged).CombinedOutput(); err != nil {
		_ = os.RemoveAll(staged)
		return "", fmt.Errorf("couldn't copy the update: %s", strings.TrimSpace(string(out)))
	}
	if info, err := os.Stat(filepath.Join(staged, "Contents", "MacOS", "Pigeonhole")); err != nil || info.Mode()&0o111 == 0 {
		_ = os.RemoveAll(staged)
		return "", errors.New("the update's app is incomplete")
	}
	return staged, nil
}

// swapScript waits for $1 to exit, replaces $2 with $3 (restoring $2 if
// that fails) and opens $2. Paths are arguments, so no quoting issues.
const swapScript = `
while kill -0 "$1" 2>/dev/null; do sleep 0.5; done
old="$2.previous"
rm -rf "$old"
if mv "$2" "$old" && mv "$3" "$2"; then
	rm -rf "$old"
elif [ -d "$old" ] && [ ! -d "$2" ]; then
	mv "$old" "$2"
fi
[ "$4" = open ] && open "$2"
`

func swapAfterExit(pid int, bundle, staged string) error {
	return startSwap(pid, bundle, staged, true)
}

func startSwap(pid int, bundle, staged string, reopen bool) error {
	open := "open"
	if !reopen {
		open = "no"
	}
	cmd := exec.Command("/bin/sh", "-c", swapScript, "pigeonhole-update", fmt.Sprint(pid), bundle, staged, open)
	// Its own session, so it outlives this process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
