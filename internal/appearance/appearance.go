// Package appearance applies the user's light/dark choice to native window
// chrome (title bars, the login window) to match the web UI's theme.
package appearance

// Theme values, as stored in settings and used by the UI.
const (
	System = "system"
	Light  = "light"
	Dark   = "dark"
)

// Valid reports whether t is a known theme.
func Valid(t string) bool {
	return t == System || t == Light || t == Dark
}
