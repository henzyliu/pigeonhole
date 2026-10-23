//go:build !darwin

package loginbrowser

// The Windows version (WebView2's cookie manager) isn't written yet; logins
// fall back to pasting cookies.
const supported = false

func nativeOpen(id int, request string) {}
func nativeClose(id int)                {}
