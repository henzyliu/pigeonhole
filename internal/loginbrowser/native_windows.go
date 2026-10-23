//go:build windows

package loginbrowser

// The Windows login window: a plain Win32 window hosting WebView2 (the Edge
// engine Wails already uses), each on its own thread with its own message
// loop. Every window gets a fresh, temporary browser profile, so it's
// isolated from the user's own Edge and from earlier logins.
//
// Like the Mac version, it only reports snapshots (URL, cookies, page values)
// every second and on each page load; Collect decides when the login is done.
//
// go-webview2's Chromium sets up the browser, but its helpers exit the whole
// process on any error, so after setup this calls the COM interfaces
// directly. Its GetCookies wrapper also treats the asynchronous call as
// synchronous, so cookies are read through our own completion handler.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/rs/zerolog"
	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

const supported = true

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procSetTimer         = user32.NewProc("SetTimer")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetDpiForSystem  = user32.NewProc("GetDpiForSystem")
	procLoadCursorW      = user32.NewProc("LoadCursorW")

	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy = 0x0002
	wmSize    = 0x0005
	wmClose   = 0x0010
	wmTimer   = 0x0113

	wsOverlappedWindow = 0x00CF0000
	swShow             = 5
	smCxScreen         = 0
	smCyScreen         = 1
	idcArrow           = 32512
	colorWindow        = 5

	snapshotTimer = 1
	windowClass   = "PigeonholeLoginWindow"
	// Temporary profiles left behind (e.g. by a crash) are removed after this.
	staleProfileAge = time.Hour
)

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

type winRect struct{ Left, Top, Right, Bottom int32 }

type loginWindow struct {
	id   int
	req  openRequest
	log  *zerolog.Logger
	hwnd uintptr

	chromium   *edge.Chromium
	controller *edge.ICoreWebView2Controller
	webview    *edge.ICoreWebView2
	cookies    *edge.ICoreWebView2CookieManager
	handler    *cookiesHandler
	ready      bool

	page           pageState
	readingCookies bool

	// closedByGo is set when Go closes the window after the login finished,
	// so it isn't reported as the user closing it. Guarded by windowsLock.
	closedByGo bool
}

var (
	windowsLock sync.Mutex
	windowsByID = map[int]*loginWindow{}
	// windowsByHwnd is only used on each window's own thread, but the map is
	// shared, so it's guarded too.
	windowsByHwnd = map[uintptr]*loginWindow{}

	registerOnce sync.Once
	registerErr  error
	wndProcPtr   = windows.NewCallback(wndProc)
)

func nativeOpen(id int, request string) {
	w := &loginWindow{id: id, log: sessionLog(id)}
	if err := json.Unmarshal([]byte(request), &w.req); err != nil {
		w.log.Err(err).Msg("Invalid login window request")
		go handleClosed(id)
		return
	}
	windowsLock.Lock()
	windowsByID[id] = w
	windowsLock.Unlock()
	go w.run()
}

func nativeClose(id int) {
	windowsLock.Lock()
	w := windowsByID[id]
	if w != nil {
		w.closedByGo = true
	}
	hwnd := uintptr(0)
	if w != nil {
		hwnd = w.hwnd
	}
	windowsLock.Unlock()
	// If the window doesn't exist yet, run() sees closedByGo once it does.
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

func sessionLog(id int) *zerolog.Logger {
	sessionsLock.Lock()
	sess := sessions[id]
	sessionsLock.Unlock()
	if sess == nil || sess.log == nil {
		nop := zerolog.Nop()
		return &nop
	}
	return sess.log
}

// run owns the window: it creates it, runs its message loop until it's
// destroyed, then cleans up. WebView2 needs a single-threaded COM apartment
// with a message loop, so it stays on one OS thread.
func (w *loginWindow) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	closedByUser := true
	var profile string
	defer func() {
		if r := recover(); r != nil {
			w.log.Error().Str("panic", fmt.Sprint(r)).Msg("Login window crashed")
		}
		w.cleanup(profile)
		windowsLock.Lock()
		closedByUser = !w.closedByGo
		delete(windowsByID, w.id)
		windowsLock.Unlock()
		if closedByUser {
			handleClosed(w.id)
		}
	}()

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		w.log.Err(err).Msg("Login window couldn't start COM")
		return
	}
	defer windows.CoUninitialize()

	if err := registerClass(); err != nil {
		w.log.Err(err).Msg("Login window couldn't register its window class")
		return
	}
	var err error
	if profile, err = newProfileDir(); err != nil {
		w.log.Err(err).Msg("Login window couldn't create a browser profile")
		return
	}
	if err = w.createWindow(); err != nil {
		w.log.Err(err).Msg("Login window couldn't be created")
		return
	}
	windowsLock.Lock()
	closing := w.closedByGo
	windowsLock.Unlock()
	if closing {
		procDestroyWindow.Call(w.hwnd)
		return
	}

	if err = w.startBrowser(profile); err != nil {
		w.log.Err(err).Msg("Login window couldn't start the browser")
		if w.hwnd != 0 {
			procDestroyWindow.Call(w.hwnd)
		}
		return
	}
	procSetTimer.Call(w.hwnd, snapshotTimer, 1000, 0)

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func registerClass() error {
	registerOnce.Do(func() {
		instance, _, _ := procGetModuleHandleW.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		name, _ := windows.UTF16PtrFromString(windowClass)
		wc := wndClassEx{
			WndProc:    wndProcPtr,
			Instance:   instance,
			Cursor:     cursor,
			Background: colorWindow + 1,
			ClassName:  name,
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			registerErr = err
		}
	})
	return registerErr
}

// newProfileDir makes a fresh browser profile folder, and removes ones left
// over from earlier windows that didn't get cleaned up.
func newProfileDir() (string, error) {
	if old, err := filepath.Glob(filepath.Join(os.TempDir(), "pigeonhole-login-*")); err == nil {
		for _, dir := range old {
			if info, err := os.Stat(dir); err == nil && time.Since(info.ModTime()) > staleProfileAge {
				_ = os.RemoveAll(dir)
			}
		}
	}
	return os.MkdirTemp("", "pigeonhole-login-")
}

func (w *loginWindow) createWindow() error {
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		dpi = 96
	}
	width, height := int32(500*dpi/96), int32(760*dpi/96)
	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x, y := (int32(screenW)-width)/2, (int32(screenH)-height)/2

	instance, _, _ := procGetModuleHandleW.Call(0)
	class, _ := windows.UTF16PtrFromString(windowClass)
	title, _ := windows.UTF16PtrFromString(w.req.Title)
	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsOverlappedWindow,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, instance, 0)
	if hwnd == 0 {
		return err
	}
	windowsLock.Lock()
	w.hwnd = hwnd
	windowsByHwnd[hwnd] = w
	windowsLock.Unlock()
	if !w.req.Hidden {
		procShowWindow.Call(hwnd, swShow)
		procSetForeground.Call(hwnd)
	}
	return nil
}

func (w *loginWindow) startBrowser(profile string) (err error) {
	c := edge.NewChromium()
	c.DataPath = profile
	c.SetErrorCallback(func(cbErr error) {
		w.log.Err(cbErr).Msg("WebView2 error in login window")
		// Ends Embed's wait when the browser can't be created at all.
		procPostQuitMessage.Call(0)
	})
	c.MessageCallback = func(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		if err := w.page.apply(message); err != nil {
			w.log.Debug().Err(err).Msg("Ignoring unexpected message from login page")
			return
		}
		w.snapshot()
	}
	c.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
		w.snapshot()
	}
	w.chromium = c

	// Embed waits for the browser to start. If the user closes the window
	// first, it stops waiting and then trips over the missing browser.
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("browser didn't start: %v", r)
			}
		}()
		c.Embed(w.hwnd)
	}()
	if err != nil {
		return err
	}
	if w.controller = c.GetController(); w.controller == nil {
		return errors.New("browser didn't start")
	}
	if w.webview, err = w.controller.GetCoreWebView2(); err != nil {
		return err
	}

	if w.req.UserAgent != "" {
		if settings, err := w.webview.GetSettings(); err == nil {
			if s2 := settings.GetICoreWebView2Settings2(); s2 != nil {
				_ = s2.PutUserAgent(w.req.UserAgent)
			}
		}
	}
	wv2, err := w.webview.QueryInterface2()
	if err != nil {
		return fmt.Errorf("WebView2 is too old to read cookies: %w", err)
	}
	w.cookies, err = wv2.GetCookieManager()
	wv2.Release()
	if err != nil {
		return err
	}
	w.handler = &cookiesHandler{vtbl: cookiesHandlerVtable, window: w}
	for _, ic := range w.req.InitialCookies {
		w.addCookie(ic.Name, ic.Value, ic.Domain, ic.Path, ic.Secure, ic.HTTPOnly)
	}
	if w.req.ExtractJS != "" {
		if err = w.webview.AddScriptToExecuteOnDocumentCreated(extractScript(w.req.ExtractJS), nil); err != nil {
			return err
		}
	}
	w.ready = true
	w.resize()
	return w.webview.Navigate(w.req.URL)
}

func (w *loginWindow) addCookie(name, value, domain, path string, secure, httpOnly bool) {
	if path == "" {
		path = "/"
	}
	cookie, err := w.cookies.CreateCookie(name, value, domain, path)
	if err != nil {
		w.log.Warn().Err(err).Str("cookie", name).Msg("Couldn't create initial cookie")
		return
	}
	defer cookie.Release()
	_ = cookie.PutIsSecure(secure)
	_ = cookie.PutIsHttpOnly(httpOnly)
	if err = w.cookies.AddOrUpdateCookie(cookie); err != nil {
		w.log.Warn().Err(err).Str("cookie", name).Msg("Couldn't set initial cookie")
	}
}

func (w *loginWindow) resize() {
	if !w.ready {
		return
	}
	var r winRect
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	_ = w.controller.PutBounds(edge.Rect{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Bottom})
}

// snapshot asks the page for its localStorage values (they arrive as a
// message and land in the next snapshot) and reads the cookies, which
// completes in onCookies.
func (w *loginWindow) snapshot() {
	if !w.ready || w.readingCookies {
		return
	}
	if len(w.req.LocalStorageKeys) > 0 {
		_ = w.webview.ExecuteScript(storageScript(w.req.LocalStorageKeys), nil)
	}
	if err := getAllCookies(w.cookies, w.handler); err != nil {
		w.log.Debug().Err(err).Msg("Couldn't read login window cookies")
		return
	}
	w.readingCookies = true
}

func (w *loginWindow) onCookies(result uint32, list *edge.ICoreWebView2CookieList) {
	w.readingCookies = false
	if !w.ready {
		return
	}
	var cookies []Cookie
	if result == 0 && list != nil {
		count, _ := list.GetCount()
		for i := uint32(0); i < count; i++ {
			item, err := list.GetItem(i)
			if err != nil || item == nil {
				continue
			}
			name, _ := item.GetName()
			value, _ := item.GetValue()
			domain, _ := item.GetDomain()
			item.Release()
			cookies = append(cookies, Cookie{Name: name, Value: value, Domain: domain})
		}
	}
	url, _ := w.webview.GetSource()
	data, err := json.Marshal(w.page.snapshot(url, cookies))
	if err != nil {
		return
	}
	handleSnapshot(w.id, string(data))
}

func (w *loginWindow) cleanup(profile string) {
	if w.ready {
		w.ready = false
		closeController(w.controller)
		w.cookies.Release()
		w.webview.Release()
	}
	if w.chromium != nil {
		w.chromium.ShuttingDown()
	}
	windowsLock.Lock()
	delete(windowsByHwnd, w.hwnd)
	windowsLock.Unlock()
	if profile != "" {
		// The browser processes take a moment to exit and release the folder.
		go func() {
			for i := 0; i < 15; i++ {
				time.Sleep(time.Second)
				if os.RemoveAll(profile) == nil {
					return
				}
			}
		}()
	}
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	windowsLock.Lock()
	w := windowsByHwnd[hwnd]
	windowsLock.Unlock()
	switch msg {
	case wmSize:
		if w != nil {
			w.resize()
		}
	case wmTimer:
		if w != nil && wParam == snapshotTimer {
			w.snapshot()
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// Raw COM, for the calls go-webview2 doesn't wrap correctly. Vtable slots
// follow WebView2.h.

// ICoreWebView2CookieManager: IUnknown (3), CreateCookie, CopyCookie, GetCookies.
const cookieManagerGetCookies = 5

// ICoreWebView2Controller: IUnknown (3), 21 property/event methods, Close.
const controllerClose = 24

func vtableSlot(object unsafe.Pointer, slot int) uintptr {
	vtbl := *(*unsafe.Pointer)(object)
	return *(*uintptr)(unsafe.Add(vtbl, slot*int(unsafe.Sizeof(uintptr(0)))))
}

// getAllCookies starts reading every cookie in the profile; the handler is
// called with the list on this thread.
func getAllCookies(mgr *edge.ICoreWebView2CookieManager, handler *cookiesHandler) error {
	empty, _ := windows.UTF16PtrFromString("")
	hr, _, _ := syscall.SyscallN(vtableSlot(unsafe.Pointer(mgr), cookieManagerGetCookies),
		uintptr(unsafe.Pointer(mgr)), uintptr(unsafe.Pointer(empty)), uintptr(unsafe.Pointer(handler)))
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}

func closeController(c *edge.ICoreWebView2Controller) {
	syscall.SyscallN(vtableSlot(unsafe.Pointer(c), controllerClose), uintptr(unsafe.Pointer(c)))
}

// cookiesHandler implements ICoreWebView2GetCookiesCompletedHandler. Each
// window keeps its own alive for as long as the browser can call it.
type cookiesHandler struct {
	vtbl   *cookiesHandlerVtbl
	window *loginWindow
}

type cookiesHandlerVtbl struct {
	QueryInterface, AddRef, Release, Invoke uintptr
}

var cookiesHandlerVtable = &cookiesHandlerVtbl{
	QueryInterface: windows.NewCallback(func(this *cookiesHandler, _ uintptr, out *uintptr) uintptr {
		*out = uintptr(unsafe.Pointer(this))
		return 0
	}),
	// The handler's lifetime is managed by Go, so reference counts are moot.
	AddRef:  windows.NewCallback(func(*cookiesHandler) uintptr { return 1 }),
	Release: windows.NewCallback(func(*cookiesHandler) uintptr { return 1 }),
	Invoke: windows.NewCallback(func(this *cookiesHandler, result uintptr, list *edge.ICoreWebView2CookieList) uintptr {
		this.window.onCookies(uint32(result), list)
		return 0
	}),
}
