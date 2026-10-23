//go:build !darwin && !windows

package loginbrowser

// Other platforms have no login window; logins fall back to pasting cookies.
const supported = false

func nativeOpen(id int, request string) {}
func nativeClose(id int)                {}
