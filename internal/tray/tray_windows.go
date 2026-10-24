//go:build windows

package tray

// The notification-area icon: the app's own icon, the unread count in its
// tooltip, a click to open Pigeonhole and a right-click menu to open or quit.
// It lives on its own thread with a hidden window to receive the icon's
// messages, like the login window.

import (
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procPostMessageW      = user32.NewProc("PostMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procRegisterWindowMsg = user32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenuW       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procSetForeground     = user32.NewProc("SetForegroundWindow")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procLoadIconW         = user32.NewProc("LoadIconW")

	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconExW   = shell32.NewProc("ExtractIconExW")

	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmClose        = 0x0010
	wmDestroy      = 0x0002
	wmNull         = 0x0000
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205
	wmApp          = 0x8000
	wmTrayIcon     = wmApp + 1 // the icon's mouse events
	wmTrayUpdate   = wmApp + 2 // refresh the tooltip from Go
	nimAdd         = 0
	nimModify      = 1
	nimDelete      = 2
	nifMessage     = 0x1
	nifIcon        = 0x2
	nifTip         = 0x4
	mfString       = 0x0
	mfSeparator    = 0x800
	tpmReturnCmd   = 0x0100
	tpmRightBtn    = 0x0002
	idiApplication = 32512
	menuOpen       = 1
	menuQuit       = 2
	windowClass    = "PigeonholeTray"
)

// notifyIconData is NOTIFYICONDATAW.
type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	TimeoutOrVer    uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            windows.GUID
	BalloonIcon     uintptr
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
	Private uint32
}

var (
	lock    sync.Mutex
	hwnd    uintptr
	icon    uintptr
	tooltip = "Pigeonhole"
	// taskbarCreated is broadcast when Explorer restarts, which drops every
	// tray icon; the icon is added again when it arrives.
	taskbarCreated uintptr
	wndProcPtr     = windows.NewCallback(wndProc)
)

func start() {
	lock.Lock()
	running := hwnd != 0
	lock.Unlock()
	if running {
		return
	}
	go run()
}

func stop() {
	lock.Lock()
	h := hwnd
	lock.Unlock()
	if h != 0 {
		procPostMessageW.Call(h, wmClose, 0, 0)
	}
}

func setUnread(n int) {
	tip := "Pigeonhole"
	if n > 0 {
		tip = "Pigeonhole: " + itoa(n) + " unread"
	}
	lock.Lock()
	tooltip = tip
	h := hwnd
	lock.Unlock()
	if h != 0 {
		procPostMessageW.Call(h, wmTrayUpdate, 0, 0)
	}
}

// hideApp isn't needed on Windows, where Wails can start the window hidden.
func hideApp() {}

func run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	instance, _, _ := procGetModuleHandleW.Call(0)
	name, _ := windows.UTF16PtrFromString(windowClass)
	wc := wndClassEx{WndProc: wndProcPtr, Instance: instance, ClassName: name}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	// A hidden top-level window (not message-only, which wouldn't get
	// Explorer's TaskbarCreated broadcast).
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)),
		0, 0, 0, 0, 0, 0, 0, instance, 0)
	if h == 0 {
		return
	}
	msgName, _ := windows.UTF16PtrFromString("TaskbarCreated")
	taskbarCreated, _, _ = procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(msgName)))
	lock.Lock()
	hwnd = h
	lock.Unlock()

	icon = appIcon()
	notify(nimAdd)

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	lock.Lock()
	hwnd = 0
	lock.Unlock()
}

// appIcon is the exe's own icon (the pigeon), or Windows' generic one.
func appIcon() uintptr {
	if exe, err := os.Executable(); err == nil {
		path, _ := windows.UTF16PtrFromString(exe)
		var small uintptr
		if n, _, _ := procExtractIconExW.Call(uintptr(unsafe.Pointer(path)), 0, 0, uintptr(unsafe.Pointer(&small)), 1); n > 0 && small != 0 {
			return small
		}
	}
	generic, _, _ := procLoadIconW.Call(0, idiApplication)
	return generic
}

func notify(action uintptr) {
	lock.Lock()
	data := notifyIconData{Wnd: hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTrayIcon, Icon: icon}
	tip, _ := windows.UTF16FromString(tooltip)
	lock.Unlock()
	copy(data.Tip[:len(data.Tip)-1], tip)
	data.Size = uint32(unsafe.Sizeof(data))
	procShellNotifyIconW.Call(action, uintptr(unsafe.Pointer(&data)))
}

func showMenu(h uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	for _, item := range []struct {
		id    uintptr
		label string
	}{{menuOpen, "Open Pigeonhole"}, {0, ""}, {menuQuit, "Quit Pigeonhole"}} {
		if item.id == 0 {
			procAppendMenuW.Call(menu, mfSeparator, 0, 0)
			continue
		}
		label, _ := windows.UTF16PtrFromString(item.label)
		procAppendMenuW.Call(menu, mfString, item.id, uintptr(unsafe.Pointer(label)))
	}
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// Without this the menu doesn't close when you click elsewhere.
	procSetForeground.Call(h)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightBtn, uintptr(pt.X), uintptr(pt.Y), 0, h, 0)
	procPostMessageW.Call(h, wmNull, 0, 0)
	switch cmd {
	case menuOpen:
		openApp()
	case menuQuit:
		quitApp()
	}
}

func wndProc(h, msg, wParam, lParam uintptr) uintptr {
	switch {
	case msg == wmTrayIcon:
		switch lParam {
		case wmLButtonUp:
			openApp()
		case wmRButtonUp:
			showMenu(h)
		}
		return 0
	case msg == wmTrayUpdate:
		notify(nimModify)
		return 0
	case taskbarCreated != 0 && msg == taskbarCreated:
		notify(nimAdd)
		return 0
	case msg == wmClose:
		notify(nimDelete)
		procDestroyWindow.Call(h)
		return 0
	case msg == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(h, msg, wParam, lParam)
	return r
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}
