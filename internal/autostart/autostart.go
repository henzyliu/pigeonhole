// Package autostart starts Pigeonhole when you log in, in the background
// (BackgroundFlag), so it's connected and notifying without opening a
// window. The operating system holds the setting; there's no copy of it in
// settings.json to drift out of date.
package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

// BackgroundFlag is passed when launched at login: start hidden.
const BackgroundFlag = "--background"

// ErrUnsupported is returned where starting at login isn't implemented.
var ErrUnsupported = errors.New("starting at login isn't supported on this system")

// Supported reports whether starting at login works here.
func Supported() bool { return supported }

// Enabled reports whether Pigeonhole starts at login.
func Enabled() bool { return enabled() }

// Set turns starting at login on or off.
func Set(on bool) error {
	if !supported {
		return ErrUnsupported
	}
	if !on {
		return disable()
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return enable(exe)
}

// Refresh points an existing login entry at this copy of the app, in case
// it has moved (or been replaced by a newer build somewhere else).
func Refresh() error {
	if !supported || !enabled() {
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return enable(exe)
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
