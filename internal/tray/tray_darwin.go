//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

void ph_tray_start(void);
void ph_tray_set_unread(int n);
void ph_tray_hide_app(void);
*/
import "C"

func start()          { C.ph_tray_start() }
func stop()           {}
func setUnread(n int) { C.ph_tray_set_unread(C.int(n)) }
func hideApp()        { C.ph_tray_hide_app() }

//export pigeonholeTrayOpen
func pigeonholeTrayOpen() { openApp() }

//export pigeonholeTrayQuit
func pigeonholeTrayQuit() { quitApp() }

func refreshTaskbar() {}
