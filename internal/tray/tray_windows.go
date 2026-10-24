//go:build windows

package tray

// The notification-area icon: the app's own icon (with a red dot when
// something's unread), the unread count in its tooltip, a click to open
// Pigeonhole and a right-click menu to open or quit. While the window is
// open, its taskbar button also gets a red badge with the count.
// It lives on its own thread with a hidden window to receive the icon's
// messages, like the login window.

import (
	"bytes"
	"image"
	_ "image/png"
	"os"
	"runtime"
	"sync"
	"syscall"
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
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procCreateIconFromRes = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon       = user32.NewProc("DestroyIcon")
	procFindWindowExW     = user32.NewProc("FindWindowExW")
	procGetWindowThreadID = user32.NewProc("GetWindowThreadProcessId")

	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

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
	wmTrayUpdate   = wmApp + 2 // refresh the icon and tooltip from Go
	wmTaskbar      = wmApp + 3 // put the taskbar badge back
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
	smCxSmIcon     = 49
	// Wails' main window class, to find Pigeonhole's taskbar button.
	wailsWindowClass = "wailsWindow"
	// ITaskbarList3::SetOverlayIcon: IUnknown (3), ITaskbarList (5),
	// ITaskbarList2 (1), then 9 ITaskbarList3 methods before it.
	taskbarHrInit         = 3
	taskbarSetOverlayIcon = 18
)

var (
	clsidTaskbarList = windows.GUID{Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11d0, Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iidTaskbarList3  = windows.GUID{Data1: 0xEA1AFB91, Data2: 0x9E28, Data3: 0x4B86, Data4: [8]byte{0x90, 0xE9, 0x9E, 0x9F, 0x8A, 0x5E, 0xEF, 0xAF}}
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
	unread  int
	// The tray icon without and with the unread dot, made on the tray
	// thread at the system's small-icon size.
	plainIcon, dotIcon uintptr
	iconSize           = 16
	// taskbar is the ITaskbarList3 used for the taskbar button's badge.
	taskbar unsafe.Pointer
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
	unread = n
	h := hwnd
	lock.Unlock()
	if h != 0 {
		procPostMessageW.Call(h, wmTrayUpdate, 0, 0)
	}
}

func refreshTaskbar() {
	lock.Lock()
	h := hwnd
	lock.Unlock()
	if h != 0 {
		procPostMessageW.Call(h, wmTaskbar, 0, 0)
	}
}

// hideApp isn't needed on Windows, where Wails can start the window hidden.
func hideApp() {}

func run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// The taskbar API is COM, used from this thread.
	if windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED) == nil {
		defer windows.CoUninitialize()
	}

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

	makeIcons()
	icon = plainIcon
	notify(nimAdd)
	initTaskbar()

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

// makeIcons draws the tray icons from the app icon PNG at the system's
// small-icon size (which grows with display scaling), falling back to the
// exe's icon without a dot.
func makeIcons() {
	if size, _, _ := procGetSystemMetrics.Call(smCxSmIcon); size > 0 {
		iconSize = int(size)
	}
	if src, _, err := image.Decode(bytes.NewReader(appIconPNG)); err == nil {
		if data, err := trayIconPNG(src, iconSize, false); err == nil {
			plainIcon = iconFromPNG(data)
		}
		if data, err := trayIconPNG(src, iconSize, true); err == nil {
			dotIcon = iconFromPNG(data)
		}
	}
	if plainIcon == 0 {
		plainIcon = appIcon()
	}
	if dotIcon == 0 {
		dotIcon = plainIcon
	}
}

func iconFromPNG(data []byte) uintptr {
	if len(data) == 0 {
		return 0
	}
	// Windows reads PNG-compressed icon images directly (version 3.0 format).
	h, _, _ := procCreateIconFromRes.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, 0x00030000,
		uintptr(iconSize), uintptr(iconSize), 0)
	return h
}

func initTaskbar() {
	var obj unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidTaskbarList3)), uintptr(unsafe.Pointer(&obj)))
	if int32(hr) < 0 || obj == nil {
		return
	}
	if r := comCall(obj, taskbarHrInit); int32(r) < 0 {
		return
	}
	taskbar = obj
}

func comCall(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, slot*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

// mainWindow finds Pigeonhole's own Wails window (another Wails app could
// use the same window class).
func mainWindow() uintptr {
	class, _ := windows.UTF16PtrFromString(wailsWindowClass)
	pid := uint32(os.Getpid())
	var h uintptr
	for {
		h, _, _ = procFindWindowExW.Call(0, h, uintptr(unsafe.Pointer(class)), 0)
		if h == 0 {
			return 0
		}
		var owner uint32
		procGetWindowThreadID.Call(h, uintptr(unsafe.Pointer(&owner)))
		if owner == pid {
			return h
		}
	}
}

// updateTaskbar sets the taskbar button's badge: the unread count, or none.
func updateTaskbar() {
	if taskbar == nil {
		return
	}
	main := mainWindow()
	if main == 0 {
		return
	}
	lock.Lock()
	n := unread
	lock.Unlock()
	var badge uintptr
	desc := ""
	if n > 0 {
		if data, err := badgePNG(n, iconSize); err == nil {
			badge = iconFromPNG(data)
		}
		desc = itoa(n) + " unread"
	}
	descPtr, _ := windows.UTF16PtrFromString(desc)
	comCall(taskbar, taskbarSetOverlayIcon, main, badge, uintptr(unsafe.Pointer(descPtr)))
	// The taskbar keeps its own copy.
	if badge != 0 {
		procDestroyIcon.Call(badge)
	}
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
		lock.Lock()
		if unread > 0 {
			icon = dotIcon
		} else {
			icon = plainIcon
		}
		lock.Unlock()
		notify(nimModify)
		updateTaskbar()
		return 0
	case msg == wmTaskbar:
		updateTaskbar()
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
