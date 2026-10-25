package update

import "errors"

// ErrCantInstall means this copy can't update itself (an unsupported
// system, or an app folder it can't write to); offer the download instead.
var ErrCantInstall = errors.New("this copy of Pigeonhole can't update itself")

// CanInstall reports whether Install can work for this copy of the app,
// before anything is downloaded.
func CanInstall() error { return canInstall() }

// Install puts a downloaded update in place and arranges for the new
// version to start once this one has quit. The caller should quit next.
func Install(file string) error { return install(file) }
