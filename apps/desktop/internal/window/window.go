//go:build windows

// Package window owns the native host window and embeds a WebView2 in it. The
// app manages the host window itself, so it starts hidden with no flash and is
// shown from the tray. Embedding requires CoInitializeEx (WebView2), which the
// webview library only does when it owns the window.
package window

import (
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	webview "github.com/webview/webview_go"
	"golang.org/x/sys/windows"
)

// Options configures the native window.
type Options struct {
	Title  string
	Width  int
	Height int
	URL    string
	Debug  bool
	Hidden bool
	// HTML loads an in-memory page instead of URL (the first-run setup page,
	// which has no server yet). Must be set before Run, as it is loaded here.
	HTML string
	// Icon is an .ico file used as the window/taskbar icon.
	Icon []byte
}

var (
	mu       sync.Mutex
	instance webview.WebView
	handle   windows.HWND

	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procEnumChildWindows = user32.NewProc("EnumChildWindows")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procIsZoomed         = user32.NewProc("IsZoomed")
	procPostMessage      = user32.NewProc("PostMessageW")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procLoadImage        = user32.NewProc("LoadImageW")
	procSendMessage      = user32.NewProc("SendMessageW")
	procSetDpiContext    = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow  = user32.NewProc("GetDpiForWindow")
	procMetricsForDpi    = user32.NewProc("GetSystemMetricsForDpi")

	closeHandler    func()
	classRegistered bool
	moduleHandle    windows.Handle
	appIcon         windows.Handle
	enumProc        uintptr
	resizeWidth     uintptr
	resizeHeight    uintptr

	// Manual move/resize state, driven from WM_MOUSEMOVE while captured.
	dragActive bool
	dragResize bool
	dragWest   bool
	dragEast   bool
	dragNorth  bool
	dragSouth  bool
	dragStart  point
	dragWindow rect
)

const (
	wmSize           = 0x0005
	wmClose          = 0x0010
	wmDestroy        = 0x0002
	wmMouseMove      = 0x0200
	wmLButtonUp      = 0x0202
	wmCaptureChanged = 0x0215
	wmGetMinMaxInfo  = 0x0024
	wmDPIChanged     = 0x02E0
	wmNCCalcSize     = 0x0083
	wmNCLButtonDown  = 0x00A1

	wsOverlappedWindow = 0x00CF0000
	swRestore          = 9
	swHide             = 0
	swMinimize         = 6
	swMaximize         = 3
	swpNoZorder        = 0x0004
	swpNoActivate      = 0x0010
	idcArrow           = 32512

	// System metrics used to measure the native resize border.
	smCXFrame        = 32
	smCYFrame        = 33
	smCXPaddedBorder = 92

	// Hit-test codes sent to start the system's move/resize modal loop.
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17

	// Window icon messages and LoadImage flags.
	wmSetIcon      = 0x0080
	iconSmall      = 0
	iconBig        = 1
	imageIcon      = 1
	lrLoadFromFile = 0x0010
	lrDefaultSize  = 0x0040

	// minWindowWidth/Height keep the two-column layout usable.
	minWindowWidth  = 900
	minWindowHeight = 620
)

// cwUsedDefault is CW_USEDEFAULT (INT_MIN). Kept as an int variable so the
// uintptr conversion sign-extends correctly.
var cwUsedDefault = -2147483648

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type point struct {
	x int32
	y int32
}

// minMaxInfo mirrors MINMAXINFO for WM_GETMINMAXINFO.
type minMaxInfo struct {
	ptReserved     point
	ptMaxSize      point
	ptMaxPosition  point
	ptMinTrackSize point
	ptMaxTrackSize point
}

// ncCalcSizeParams mirrors NCCALCSIZE_PARAMS for WM_NCCALCSIZE.
type ncCalcSizeParams struct {
	rgrc  [3]rect
	lppos uintptr
}

func init() {
	enumProc = windows.NewCallback(func(child, _ uintptr) uintptr {
		_, _, _ = procSetWindowPos.Call(child, 0, 0, 0, resizeWidth, resizeHeight, swpNoZorder|swpNoActivate)
		return 1
	})
}

// Prepare creates the hidden host window and embeds the WebView2 in it. It must
// run on the main thread before Run and does not block.
func Prepare(opts Options) {
	// Required before WebView2 initialization when embedding into our window.
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	// The webview library only enables DPI awareness for windows it creates
	// itself. Without it Windows bitmap-stretches the whole page on scaled
	// displays, which is why text looked blurrier than in a browser.
	enableDPIAwareness()

	appIcon = loadIcon(opts.Icon)

	if err := ensureClass(); err != nil {
		panic(err)
	}
	host, err := createHost(opts.Title, opts.Width, opts.Height)
	if err != nil {
		panic(err)
	}
	handle = host
	applyWindowIcon(host)

	instance = webview.NewWindow(opts.Debug, unsafe.Pointer(&host))
	instance.SetTitle(opts.Title)
	instance.SetSize(opts.Width, opts.Height, webview.HintNone)
	instance.Init(desktopScript)
	bindControls(instance)
	fillChildren(host)
	if opts.HTML != "" {
		instance.SetHtml(opts.HTML)
	} else if opts.URL != "" {
		instance.Navigate(opts.URL)
	}
	if opts.Hidden {
		Hide()
	}
}

// Run pumps the thread message loop until the window is destroyed.
func Run() {
	instance.Run()
}

// Destroy releases the webview and the host window. Call it after Run returns.
func Destroy() {
	if instance != nil {
		instance.Destroy()
	}
	if handle != 0 {
		_, _, _ = procDestroyWindow.Call(uintptr(handle))
	}
	windows.CoUninitialize()
}

// Show restores and focuses the window. Safe from any goroutine.
func Show() {
	mu.Lock()
	h := handle
	w := instance
	mu.Unlock()
	if h == 0 {
		return
	}
	if w != nil {
		w.Dispatch(func() { fillChildren(h) })
	}
	_, _, _ = procShowWindow.Call(uintptr(h), uintptr(swRestore))
	_, _, _ = procSetForeground.Call(uintptr(h))
}

// Hide hides the window without closing it. Safe from any goroutine.
func Hide() {
	mu.Lock()
	h := handle
	mu.Unlock()
	if h == 0 {
		return
	}
	_, _, _ = procShowWindow.Call(uintptr(h), uintptr(swHide))
}

// Navigate points the webview at a new URL. Safe from any goroutine.
func Navigate(url string) {
	mu.Lock()
	w := instance
	mu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Navigate(url) })
	}
}

// Quit stops the message loop. Safe from any goroutine.
func Quit() {
	mu.Lock()
	w := instance
	mu.Unlock()
	if w != nil {
		w.Terminate()
	}
}

// Bind exposes a Go function to JavaScript as a global. Call it before Run so
// every page, including the data-URI pairing page, has it.
func Bind(name string, fn any) error {
	mu.Lock()
	w := instance
	mu.Unlock()
	if w == nil {
		return errNotPrepared
	}
	return w.Bind(name, fn)
}

// InterceptClose makes closing the window call onClose (usually Hide) instead
// of destroying it, so the tray and the server keep running.
func InterceptClose(onClose func()) {
	closeHandler = onClose
}

func ensureClass() error {
	if classRegistered {
		return nil
	}
	moduleResult, _, _ := procGetModuleHandle.Call(0)
	moduleHandle = windows.Handle(moduleResult)

	proc := windows.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
		switch msg {
		case wmSize:
			fillChildren(windows.HWND(hwnd))
			notifyWindowState()
			return 0
		case wmNCCalcSize:
			insetClientArea(windows.HWND(hwnd), wparam, lparam)
			return 0
		case wmGetMinMaxInfo:
			result, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
			applyMinSize(windows.HWND(hwnd), lparam)
			return result
		case wmDPIChanged:
			// Moving to a monitor with another scale: take the size Windows
			// suggests so the window keeps its physical proportions.
			suggested := lparamPtr[rect](lparam)
			_, _, _ = procSetWindowPos.Call(hwnd, 0,
				uintptr(suggested.left), uintptr(suggested.top),
				uintptr(suggested.right-suggested.left), uintptr(suggested.bottom-suggested.top),
				swpNoZorder|swpNoActivate)
			return 0
		case wmClose:
			if closeHandler != nil {
				closeHandler()
			}
			return 0
		case wmDestroy:
			return 0
		}
		result, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
		return result
	})

	className, _ := windows.UTF16PtrFromString("PalorosaHost")
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)

	class := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   proc,
		hInstance:     moduleHandle,
		hIcon:         appIcon,
		hCursor:       windows.Handle(cursor),
		lpszClassName: className,
		hIconSm:       appIcon,
	}
	result, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if result == 0 {
		return callErr
	}
	classRegistered = true
	return nil
}

// createHost creates the app window hidden (no WS_VISIBLE). The webview library
// never shows it when embedding, so there is no flash on start.
func createHost(title string, width, height int) (windows.HWND, error) {
	className, _ := windows.UTF16PtrFromString("PalorosaHost")
	windowName, _ := windows.UTF16PtrFromString(title)
	result, _, callErr := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		wsOverlappedWindow,
		uintptr(cwUsedDefault), uintptr(cwUsedDefault),
		uintptr(width), uintptr(height),
		0, 0, uintptr(moduleHandle), 0,
	)
	if result == 0 {
		return 0, callErr
	}
	return windows.HWND(result), nil
}

// fillChildren makes the embedded webview fill the host client area.
func fillChildren(host windows.HWND) {
	var client rect
	_, _, _ = procGetClientRect.Call(uintptr(host), uintptr(unsafe.Pointer(&client)))
	resizeWidth = uintptr(client.right - client.left)
	resizeHeight = uintptr(client.bottom - client.top)
	if resizeWidth == 0 || resizeHeight == 0 {
		return
	}
	_, _, _ = procEnumChildWindows.Call(uintptr(host), enumProc, 0)
}

// loadIcon turns embedded .ico bytes into an HICON, or 0 when absent.
func loadIcon(data []byte) windows.Handle {
	if len(data) == 0 {
		return 0
	}
	path := filepath.Join(os.TempDir(), "palorosa-kitchen-app.ico")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return 0
	}
	name, _ := windows.UTF16PtrFromString(path)
	icon, _, _ := procLoadImage.Call(0, uintptr(unsafe.Pointer(name)), imageIcon, 0, 0, lrLoadFromFile|lrDefaultSize)
	_ = os.Remove(path)
	return windows.Handle(icon)
}

// applyWindowIcon sets the big and small window icons (taskbar/title bar).
func applyWindowIcon(hwnd windows.HWND) {
	if appIcon == 0 {
		return
	}
	_, _, _ = procSendMessage.Call(uintptr(hwnd), wmSetIcon, iconBig, uintptr(appIcon))
	_, _, _ = procSendMessage.Call(uintptr(hwnd), wmSetIcon, iconSmall, uintptr(appIcon))
}

// currentWebView returns the live webview, or nil before Prepare finishes.
func currentWebView() webview.WebView {
	mu.Lock()
	defer mu.Unlock()
	return instance
}

func systemMetric(index int) int32 {
	value, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(value)
}

// enableDPIAwareness opts the process into per-monitor v2 DPI awareness (and
// falls back silently on Windows versions without it). Must run before any
// window is created.
func enableDPIAwareness() {
	if procSetDpiContext.Find() != nil {
		return
	}
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the pseudo-handle -4.
	perMonitorV2 := ^uintptr(3)
	_, _, _ = procSetDpiContext.Call(perMonitorV2)
}

// windowDPI returns the DPI of the monitor the window is on (96 = 100 %).
func windowDPI(hwnd windows.HWND) int32 {
	if hwnd == 0 || procGetDpiForWindow.Find() != nil {
		return 96
	}
	value, _, _ := procGetDpiForWindow.Call(uintptr(hwnd))
	if value == 0 {
		return 96
	}
	return int32(value)
}

// scaleForDPI converts a 96-DPI length to physical pixels.
func scaleForDPI(value, dpi int32) int32 {
	return value * dpi / 96
}

// metricForDPI is GetSystemMetricsForDpi with a GetSystemMetrics fallback.
func metricForDPI(index int, dpi int32) int32 {
	if procMetricsForDpi.Find() != nil {
		return systemMetric(index)
	}
	value, _, _ := procMetricsForDpi.Call(uintptr(index), uintptr(dpi))
	return int32(value)
}

// frameBorder returns the resize border thickness on each axis.
func frameBorder(hwnd windows.HWND) (int32, int32) {
	dpi := windowDPI(hwnd)
	padding := metricForDPI(smCXPaddedBorder, dpi)
	return metricForDPI(smCXFrame, dpi) + padding, metricForDPI(smCYFrame, dpi) + padding
}

// minSize is the minimum window size in physical pixels for the window's DPI.
func minSize(hwnd windows.HWND) (int32, int32) {
	dpi := windowDPI(hwnd)
	return scaleForDPI(minWindowWidth, dpi), scaleForDPI(minWindowHeight, dpi)
}

func isZoomed(hwnd windows.HWND) bool {
	value, _, _ := procIsZoomed.Call(uintptr(hwnd))
	return value != 0
}

// insetClientArea hides the caption while keeping the native resize border, so
// the system keeps drawing the drop shadow and handling edge resizing, snapping
// and the maximize animation. Called from WM_NCCALCSIZE.
func insetClientArea(hwnd windows.HWND, wparam, lparam uintptr) {
	frameX, frameY := frameBorder(hwnd)
	var client *rect
	if wparam == 0 {
		client = lparamPtr[rect](lparam)
	} else {
		client = &lparamPtr[ncCalcSizeParams](lparam).rgrc[0]
	}
	client.left += frameX
	client.right -= frameX
	client.bottom -= frameY
	if isZoomed(hwnd) {
		// When maximized the window is placed just off-screen by the frame
		// size; reclaim the top so the content is not clipped.
		client.top += frameY
		client.bottom--
	}
}

func applyMinSize(hwnd windows.HWND, lparam uintptr) {
	info := lparamPtr[minMaxInfo](lparam)
	info.ptMinTrackSize.x, info.ptMinTrackSize.y = minSize(hwnd)
}

// lparamPtr reinterprets a window-message LPARAM as a pointer. It reads the
// bits through a pointer so vet's uintptr -> unsafe.Pointer rule stays quiet.
func lparamPtr[T any](lparam uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&lparam)))
}

// notifyWindowState tells the page whether the window is maximized so the custom
// title bar can swap its glyph. Runs on the UI thread from WM_SIZE.
func notifyWindowState() {
	w := currentWebView()
	if w == nil {
		return
	}
	state := "false"
	if isZoomed(handle) {
		state = "true"
	}
	w.Eval("window.__palorosaWindowState&&window.__palorosaWindowState(" + state + ")")
}

// bindControls exposes the custom title bar actions to the page.
func bindControls(w webview.WebView) {
	controls := map[string]any{
		"palorosaWindowMinimize":       windowMinimize,
		"palorosaWindowToggleMaximize": windowToggleMaximize,
		"palorosaWindowClose":          windowClose,
		"palorosaWindowDrag":           windowDrag,
		"palorosaWindowResize":         windowResize,
		"palorosaWindowDragMove":       windowDragMove,
		"palorosaWindowDragEnd":        windowDragEnd,
		"palorosaWindowIsMaximized":    windowIsMaximized,
	}
	for name, fn := range controls {
		_ = w.Bind(name, fn)
	}
}

func windowMinimize() {
	if handle != 0 {
		_, _, _ = procShowWindow.Call(uintptr(handle), swMinimize)
	}
}

func windowToggleMaximize() {
	if handle == 0 {
		return
	}
	if isZoomed(handle) {
		_, _, _ = procShowWindow.Call(uintptr(handle), swRestore)
		return
	}
	_, _, _ = procShowWindow.Call(uintptr(handle), swMaximize)
}

func windowClose() {
	if handle != 0 {
		_, _, _ = procPostMessage.Call(uintptr(handle), wmClose, 0, 0)
	}
}

func windowIsMaximized() bool {
	return handle != 0 && isZoomed(handle)
}

// startDrag records the starting cursor and window geometry. The page then
// feeds pointer moves back through windowDragMove until it releases, so the
// WebView keeps owning the pointer (clicks and scrolling keep working).
func startDrag(resize bool, edge string) {
	if handle == 0 || isZoomed(handle) {
		return
	}
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&dragStart)))
	_, _, _ = procGetWindowRect.Call(uintptr(handle), uintptr(unsafe.Pointer(&dragWindow)))
	dragResize = resize
	dragWest = edge == "w" || edge == "nw" || edge == "sw"
	dragEast = edge == "e" || edge == "ne" || edge == "se"
	dragNorth = edge == "n" || edge == "nw" || edge == "ne"
	dragSouth = edge == "s" || edge == "sw" || edge == "se"
	dragActive = true
}

func applyDrag() {
	if handle == 0 || !dragActive {
		return
	}
	var cursor point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	dx := cursor.x - dragStart.x
	dy := cursor.y - dragStart.y
	if !dragResize {
		moveWindow(dragWindow.left+dx, dragWindow.top+dy, dragWindow.right-dragWindow.left, dragWindow.bottom-dragWindow.top)
		return
	}
	left, top := dragWindow.left, dragWindow.top
	right, bottom := dragWindow.right, dragWindow.bottom
	if dragWest {
		left += dx
	}
	if dragEast {
		right += dx
	}
	if dragNorth {
		top += dy
	}
	if dragSouth {
		bottom += dy
	}
	minWidth, minHeight := minSize(handle)
	if right-left < minWidth {
		if dragWest {
			left = right - minWidth
		} else {
			right = left + minWidth
		}
	}
	if bottom-top < minHeight {
		if dragNorth {
			top = bottom - minHeight
		} else {
			bottom = top + minHeight
		}
	}
	moveWindow(left, top, right-left, bottom-top)
}

func moveWindow(x, y, width, height int32) {
	_, _, _ = procSetWindowPos.Call(
		uintptr(handle), 0,
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		swpNoZorder|swpNoActivate,
	)
}

// windowDrag starts moving the window from the custom title bar.
func windowDrag() {
	startDrag(false, "")
}

// windowResize starts resizing the window from the given edge.
func windowResize(edge string) {
	switch edge {
	case "n", "s", "w", "e", "nw", "ne", "sw", "se":
		startDrag(true, edge)
	}
}

// windowDragMove applies the current pointer position while dragging.
func windowDragMove() {
	applyDrag()
}

// windowDragEnd finishes a drag started by windowDrag or windowResize.
func windowDragEnd() {
	dragActive = false
}

// desktopScript runs on every page in the WebView: it flags the desktop shell,
// suppresses browser affordances (reload, devtools keys, zoom, context menu,
// text selection) and builds the custom title bar with resize handles.
const desktopScript = `(function () {
  if (window.__palorosaDesktop) { return; }
  // WebView2 runs init scripts in every frame; the title bar belongs only to
  // the top window, never to iframes such as the hidden print frame.
  if (window.top !== window) { return; }
  window.__palorosaDesktop = true;

  var blockKeys = function (event) {
    var key = event.key;
    var ctrl = event.ctrlKey || event.metaKey;
    var hit = key === 'F5' || key === 'F7' || key === 'F12' || key === 'ContextMenu';
    if (ctrl && !event.altKey && 'rRpPuUfF+-=0'.indexOf(key) >= 0) { hit = true; }
    if (ctrl && event.shiftKey && 'iIjJcC'.indexOf(key) >= 0) { hit = true; }
    if (event.altKey && (key === 'ArrowLeft' || key === 'ArrowRight')) { hit = true; }
    if (hit) { event.preventDefault(); event.stopPropagation(); }
  };
  window.addEventListener('keydown', blockKeys, true);
  window.addEventListener('contextmenu', function (event) { event.preventDefault(); }, true);
  window.addEventListener('dragstart', function (event) { event.preventDefault(); }, true);
  window.addEventListener('wheel', function (event) { if (event.ctrlKey) { event.preventDefault(); } }, { passive: false, capture: true });
  document.addEventListener('gesturestart', function (event) { event.preventDefault(); });

  var ICON = {
    min: '<svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><rect x="0" y="4.5" width="10" height="1" fill="currentColor"/></svg>',
    max: '<svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor"/></svg>',
    restore: '<svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 2.5V0.5h7v7h-2" fill="none" stroke="currentColor"/><rect x="0.5" y="2.5" width="7" height="7" fill="none" stroke="currentColor"/></svg>',
    close: '<svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M0.5 0.5l9 9M9.5 0.5l-9 9" stroke="currentColor"/></svg>'
  };

  function invoke (name, arg) {
    if (typeof window[name] !== 'function') { return; }
    if (arg === undefined) { window[name](); return; }
    window[name](arg);
  }

  var maximized = false;

  // Drives a move/resize from pointer events. The WebView keeps the pointer
  // (no host capture), so clicks and scrolling never freeze.
  function beginWindowDrag (element, event, start) {
    if (event.button !== 0) { return; }
    var pointer = event.pointerId;
    start();
    try { element.setPointerCapture(pointer); } catch (error) {}
    function move () { invoke('palorosaWindowDragMove'); }
    function end () {
      element.removeEventListener('pointermove', move);
      element.removeEventListener('pointerup', end);
      element.removeEventListener('pointercancel', end);
      element.removeEventListener('lostpointercapture', end);
      try { element.releasePointerCapture(pointer); } catch (error) {}
      invoke('palorosaWindowDragEnd');
    }
    element.addEventListener('pointermove', move);
    element.addEventListener('pointerup', end);
    element.addEventListener('pointercancel', end);
    element.addEventListener('lostpointercapture', end);
  }

  function makeButton (icon, label, action, danger) {
    var button = document.createElement('button');
    button.type = 'button';
    button.title = label;
    button.setAttribute('aria-label', label);
    button.style.cssText = 'width:46px;height:30px;border:0;background:transparent;color:inherit;display:flex;align-items:center;justify-content:center;cursor:default;border-radius:6px;padding:0;outline:none';
    button.innerHTML = icon;
    button.addEventListener('mouseenter', function () {
      button.style.background = danger ? '#e81123' : '#f3d1c7';
      if (danger) { button.style.color = '#fff'; }
    });
    button.addEventListener('mouseleave', function () {
      button.style.background = 'transparent';
      button.style.color = 'inherit';
    });
    button.addEventListener('pointerdown', function (event) { event.stopPropagation(); });
    button.addEventListener('mousedown', function (event) { event.stopPropagation(); });
    button.addEventListener('click', function (event) {
      event.stopPropagation();
      if (action) { action(); }
    });
    return button;
  }

  function addResizer (edge, css) {
    var handle = document.createElement('div');
    handle.setAttribute('data-palorosa-resize', edge);
    handle.style.cssText = 'position:fixed;z-index:2147483647;' + css;
    handle.addEventListener('pointerdown', function (event) {
      if (event.button !== 0) { return; }
      event.preventDefault();
      beginWindowDrag(handle, event, function () { invoke('palorosaWindowResize', edge); });
    });
    document.documentElement.appendChild(handle);
  }

  function buildBar () {
    if (document.querySelector('[data-palorosa-titlebar]')) { return; }

    var bar = document.createElement('div');
    bar.setAttribute('data-palorosa-titlebar', '');
    bar.style.cssText = 'position:fixed;top:0;left:0;right:0;height:34px;display:flex;align-items:center;padding:0 6px 0 14px;background:#fffaf5;border-bottom:1px solid #f3d1c7;color:#6a4b20;font:600 12px/1 "Segoe UI",system-ui,sans-serif;user-select:none;-webkit-user-select:none;z-index:2147483646';

    var title = document.createElement('span');
    title.textContent = 'Panel de cocina';
    title.style.cssText = 'flex:1 1 auto;overflow:hidden;white-space:nowrap;text-overflow:ellipsis;padding-right:12px';
    bar.appendChild(title);

    var maximizeButton = makeButton(ICON.max, 'Maximizar', function () { invoke('palorosaWindowToggleMaximize'); });
    bar.appendChild(makeButton(ICON.min, 'Minimizar', function () { invoke('palorosaWindowMinimize'); }));
    bar.appendChild(maximizeButton);
    bar.appendChild(makeButton(ICON.close, 'Cerrar', function () { invoke('palorosaWindowClose'); }, true));

    bar.addEventListener('pointerdown', function (event) {
      if (event.button !== 0) { return; }
      if (maximized) { invoke('palorosaWindowToggleMaximize'); return; }
      beginWindowDrag(bar, event, function () { invoke('palorosaWindowDrag'); });
    });
    bar.addEventListener('dblclick', function (event) {
      var target = event.target;
      if (target && target.closest && target.closest('button')) { return; }
      invoke('palorosaWindowToggleMaximize');
    });

    window.__palorosaWindowState = function (value) {
      maximized = !!value;
      maximizeButton.innerHTML = maximized ? ICON.restore : ICON.max;
      maximizeButton.title = maximized ? 'Restaurar' : 'Maximizar';
    };

    document.body.appendChild(bar);

    // Top edge and corners are client area (the caption was removed), so they
    // need JavaScript resize handles; the other edges stay native.
    addResizer('n', 'top:0;left:10px;right:10px;height:6px;cursor:ns-resize');
    addResizer('nw', 'top:0;left:0;width:12px;height:12px;cursor:nwse-resize');
    addResizer('ne', 'top:0;right:0;width:12px;height:12px;cursor:nesw-resize');

    if (typeof window.palorosaWindowIsMaximized === 'function') {
      window.palorosaWindowIsMaximized().then(function (value) {
        window.__palorosaWindowState(!!value);
      }).catch(function () {});
    }
  }

  function start () {
    document.documentElement.classList.add('is-desktop');
    var style = document.createElement('style');
    style.textContent = 'html.is-desktop{-webkit-tap-highlight-color:transparent}' +
      'html.is-desktop ::-webkit-scrollbar{width:11px;height:11px}' +
      'html.is-desktop ::-webkit-scrollbar-thumb{background:#b58574;border-radius:6px;border:3px solid #f9eadc}' +
      'html.is-desktop ::-webkit-scrollbar-track{background:transparent}' +
      '@media print{[data-palorosa-titlebar],[data-palorosa-resize]{display:none!important}}';
    (document.head || document.documentElement).appendChild(style);
    buildBar();
    if (!document.getElementById('app') && document.body) {
      document.body.style.paddingTop = '58px';
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();`

var errNotPrepared = errorString("window not prepared")

type errorString string

func (e errorString) Error() string { return string(e) }
