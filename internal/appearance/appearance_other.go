//go:build !darwin

package appearance

// Apply is a no-op here; on Windows the app uses Wails' WindowSet*Theme.
func Apply(theme string) {}
