//go:build windows

package main

import _ "embed"

// appIconPNG is drawn into the notification-area icon (see internal/tray).
//
//go:embed build/appicon.png
var appIconPNG []byte
