package window

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/lixenwraith/terminal"
	"golang.org/x/sys/windows"
)

// native is the Win32 window, its device context and what its procedure keeps
// between messages
type native struct {
	hwnd, dc uintptr
	info     bitmapInfo      // the raster's frame as GDI reads it
	scale    float64         // Config.Scale, which the display's DPI multiplies into the density
	placed   bool            // Open has reported the size: a resize from here on is posted
	full     bool            // borderless over its monitor
	before   windowPlacement // where it was before that

	// The key last gone down, as its scan code with the extended bit, and whether
	// the characters TranslateMessage queues behind it are its press
	down  uint32
	typed bool
	high  rune // a surrogate pair's first half, its second to come

	wheelV, wheelH int // wheel movement short of a step
}

// The system libraries, loaded from System32 alone, and every entry point taken
// from them, which load finds before any is called
var (
	procs  []*windows.LazyProc
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	kernel = windows.NewLazySystemDLL("kernel32.dll")

	procSetProcessDpiAwarenessContext = proc(user32, "SetProcessDpiAwarenessContext")
	procAdjustWindowRectExForDpi      = proc(user32, "AdjustWindowRectExForDpi")
	procGetDpiForWindow               = proc(user32, "GetDpiForWindow")
	procRegisterClassExW              = proc(user32, "RegisterClassExW")
	procLoadCursorW                   = proc(user32, "LoadCursorW")
	procCreateWindowExW               = proc(user32, "CreateWindowExW")
	procDestroyWindow                 = proc(user32, "DestroyWindow")
	procDefWindowProcW                = proc(user32, "DefWindowProcW")
	procShowWindow                    = proc(user32, "ShowWindow")
	procSetWindowPos                  = proc(user32, "SetWindowPos")
	procGetWindowRect                 = proc(user32, "GetWindowRect")
	procGetClientRect                 = proc(user32, "GetClientRect")
	procScreenToClient                = proc(user32, "ScreenToClient")
	procGetWindowLongPtrW             = proc(user32, "GetWindowLongPtrW")
	procSetWindowLongPtrW             = proc(user32, "SetWindowLongPtrW")
	procGetWindowPlacement            = proc(user32, "GetWindowPlacement")
	procSetWindowPlacement            = proc(user32, "SetWindowPlacement")
	procMonitorFromWindow             = proc(user32, "MonitorFromWindow")
	procGetMonitorInfoW               = proc(user32, "GetMonitorInfoW")
	procGetMessageW                   = proc(user32, "GetMessageW")
	procGetMessagePos                 = proc(user32, "GetMessagePos")
	procPeekMessageW                  = proc(user32, "PeekMessageW")
	procTranslateMessage              = proc(user32, "TranslateMessage")
	procDispatchMessageW              = proc(user32, "DispatchMessageW")
	procPostMessageW                  = proc(user32, "PostMessageW")
	procPostQuitMessage               = proc(user32, "PostQuitMessage")
	procInvalidateRect                = proc(user32, "InvalidateRect")
	procValidateRect                  = proc(user32, "ValidateRect")
	procGetDC                         = proc(user32, "GetDC")
	procGetKeyState                   = proc(user32, "GetKeyState")
	procMapVirtualKeyW                = proc(user32, "MapVirtualKeyW")
	procSetCapture                    = proc(user32, "SetCapture")
	procReleaseCapture                = proc(user32, "ReleaseCapture")
	procOpenClipboard                 = proc(user32, "OpenClipboard")
	procCloseClipboard                = proc(user32, "CloseClipboard")
	procGetClipboardData              = proc(user32, "GetClipboardData")
	procGlobalLock                    = proc(kernel, "GlobalLock")
	procGlobalUnlock                  = proc(kernel, "GlobalUnlock")
	procStretchDIBits                 = proc(gdi32, "StretchDIBits")
	procSetStretchBltMode             = proc(gdi32, "SetStretchBltMode")
	procPatBlt                        = proc(gdi32, "PatBlt")
)

func proc(dll *windows.LazyDLL, name string) *windows.LazyProc {
	p := dll.NewProc(name)
	procs = append(procs, p)
	return p
}

// Window messages, wmStop the window's own, which Close posts
const (
	wmSize, wmActivate, wmKillFocus, wmPaint, wmClose, wmEraseBkgnd = 0x0005, 0x0006, 0x0008, 0x000F, 0x0010, 0x0014
	wmKeyDown, wmKeyUp, wmChar, wmDeadChar                          = 0x0100, 0x0101, 0x0102, 0x0103
	wmSysKeyDown, wmSysKeyUp, wmSysChar                             = 0x0104, 0x0105, 0x0106
	wmMouseMove, wmLButtonDown, wmLButtonUp                         = 0x0200, 0x0201, 0x0202
	wmRButtonDown, wmRButtonUp, wmMButtonDown, wmMButtonUp          = 0x0204, 0x0205, 0x0207, 0x0208
	wmMouseWheel, wmXButtonDown, wmXButtonUp, wmMouseHWheel         = 0x020A, 0x020B, 0x020C, 0x020E
	wmEnterMenuLoop, wmSizing, wmEnterSizeMove, wmDpiChanged        = 0x0211, 0x0214, 0x0231, 0x02E0
	wmStop                                                          = 0x8000 // WM_APP
)

// Arguments and flags of the calls above, under their Win32 names
const (
	csOwnDC, idcArrow, cwUseDefault       = 0x0020, 32512, 0x80000000
	wsOverlappedWindow, wsPopup           = 0x00CF0000, 0x80000000
	swpNoSize, swpNoMove, swpNoZOrder     = 0x0001, 0x0002, 0x0004
	swpNoActivate, swpFrameChanged        = 0x0010, 0x0020
	sizeMaximized, monitorNearest         = 2, 2
	pmNoRemove, mapvkVKToChar, wheelDelta = 0, 2, 120
	mkLButton, mkRButton, mkMButton       = 0x0001, 0x0002, 0x0010
	mkButtons                             = mkLButton | mkRButton | mkMButton | 0x0020 | 0x0040 // and the X buttons
	cfUnicodeText, biBitfields, srcCopy   = 13, 3, 0x00CC0020
	blackness, colorOnColor               = 0x00000042, 3
	gwlStyle                              = ^uintptr(15) // -16
	dpiPerMonitorV2                       = ^uintptr(3)  // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, -4
)

// wndClassEx is WNDCLASSEXW
type wndClassEx struct {
	size, style        uint32
	wndProc            uintptr
	clsExtra, wndExtra int32
	instance           uintptr
	icon, cursor       uintptr
	background         uintptr
	menuName           *uint16
	className          *uint16
	iconSm             uintptr
}

// msg is MSG
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	_       uint32
}

// monitorInfo is MONITORINFO: a monitor's rectangle, and what its taskbar leaves
type monitorInfo struct {
	size          uint32
	monitor, work windows.Rect
	flags         uint32
}

// windowPlacement is WINDOWPLACEMENT
type windowPlacement struct {
	length, flags, showCmd uint32
	min, max               [2]int32
	normal                 windows.Rect
}

// bitmapInfo is BITMAPINFOHEADER and the color masks that follow it: they name the
// raster's bytes, R, G, B, A, so GDI reads its frame where it lies
type bitmapInfo struct {
	size          uint32
	width, height int32
	planes, bits  uint16
	compression   uint32
	sizeImage     uint32
	_             [4]uint32 // resolution and palette counts
	masks         [3]uint32
}

// at is what a message's parameter or a call's result points to
func at[T any](p uintptr) *T { return *(**T)(unsafe.Pointer(&p)) }

var loaded struct {
	sync.Once
	err    error
	class  *uint16
	module uintptr
}

// live are the windows by handle, for the one procedure of their class
var live = struct {
	sync.RWMutex
	windows map[uintptr]*Window
}{windows: map[uintptr]*Window{}}

// load finds every entry point, so a system without one is an error and never a
// panic, makes the process aware of each monitor's DPI and registers the window's
// class, once
func load() error {
	loaded.Do(func() { loaded.err = register() })
	return loaded.err
}

func register() error {
	for _, p := range procs {
		if err := p.Find(); err != nil {
			return fmt.Errorf("window: %w", err)
		}
	}
	// Before any window is made, so each is sized in its monitor's own pixels; a
	// process that has chosen its awareness refuses this one, which changes nothing
	procSetProcessDpiAwarenessContext.Call(dpiPerMonitorV2)
	var module windows.Handle
	windows.GetModuleHandleEx(0, nil, &module)
	loaded.module = uintptr(module)
	loaded.class, _ = windows.UTF16PtrFromString("terminal.window")
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	// A device context of its own keeps the stretch mode open sets
	class := wndClassEx{style: csOwnDC, wndProc: windows.NewCallback(wndProc), instance: loaded.module,
		cursor: cursor, className: loaded.class}
	class.size = uint32(unsafe.Sizeof(class))
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return fmt.Errorf("window: RegisterClassExW: %w", err)
	}
	return nil
}

// run opens the window, reports to opened once it is on screen, and is its thread,
// taking its messages until Close
func (w *Window) run(cfg Config, opened chan<- error) {
	defer close(w.done)
	// A window's messages come to the thread that made it
	runtime.LockOSThread()
	if err := w.open(cfg); err != nil {
		opened <- err
		return
	}
	defer w.release()
	opened <- nil
	go w.relay()
	var m msg
	for {
		// 0 is the WM_QUIT wmStop asks for, -1 an error
		if r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// relay hands the thread what other goroutines leave for the window, since its
// loop waits on messages alone. A frame is a paint the window owes, which the
// system delivers after any input, so frames never hold keys back.
func (w *Window) relay() {
	for {
		select {
		case <-w.stop:
			procPostMessageW.Call(w.hwnd, wmStop, 0, 0)
			return
		case <-w.ready:
			procInvalidateRect.Call(w.hwnd, 0, 0)
		case a := <-w.asks:
			w.keys.Ask(a.report, a.allow, true) // Windows tells a repeat from a press
		}
	}
}

// open makes the window on the monitor the system places it on, sizes it for the
// grid at that monitor's DPI and shows it; its size is then the one on screen
func (w *Window) open(cfg Config) error {
	title, _ := windows.UTF16PtrFromString(cfg.Title)
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(loaded.class)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow, cwUseDefault, cwUseDefault, cwUseDefault, cwUseDefault, 0, 0, loaded.module, 0)
	if hwnd == 0 {
		return fmt.Errorf("window: CreateWindowExW: %w", err)
	}
	w.hwnd, w.down, w.scale = hwnd, noKey, w.density
	w.dc, _, _ = procGetDC.Call(hwnd)
	procSetStretchBltMode.Call(w.dc, colorOnColor) // a pixel stretched is a block of itself
	w.info = bitmapInfo{size: 40, planes: 1, bits: 32, compression: biBitfields, masks: [3]uint32{0xff, 0xff00, 0xff0000}}
	live.Lock()
	live.windows[hwnd] = w
	live.Unlock()
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	w.rescaled(dpi)

	// The frame around the grid's pixels, and the window no larger than its
	// monitor's work area holds in whole cells, moved only as far as fits it there
	frame := windows.Rect{}
	procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&frame)), wsOverlappedWindow, 0, 0, dpi)
	fw, fh := int(frame.Right-frame.Left), int(frame.Bottom-frame.Top)
	var place windows.Rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&place)))
	work := w.monitor().work
	width := fw + max(min(cfg.Cols, (int(work.Right-work.Left)-fw)/w.cw), 1)*w.cw
	height := fh + max(min(cfg.Rows, (int(work.Bottom-work.Top)-fh)/w.ch), 1)*w.ch
	x := max(min(int(place.Left), int(work.Right)-width), int(work.Left))
	y := max(min(int(place.Top), int(work.Bottom)-height), int(work.Top))
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoZOrder|swpNoActivate)
	var client windows.Rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	w.fit(int(client.Right), int(client.Bottom))

	// Shown, it has the size its state gives it before ShowWindow returns
	show := uintptr(windows.SW_SHOWNORMAL)
	if cfg.Fullscreen {
		w.fullscreen(true)
		show = windows.SW_SHOW
	} else if cfg.Maximized {
		show = windows.SW_MAXIMIZE
	}
	procShowWindow.Call(hwnd, show)
	w.placed = true
	return nil
}

// release destroys the window
func (w *Window) release() {
	procDestroyWindow.Call(w.hwnd)
	live.Lock()
	delete(live.windows, w.hwnd)
	live.Unlock()
}

// rescaled sizes the cell for a display of dpi dots an inch, 96 of them a dp and
// the least Windows has
func (w *Window) rescaled(dpi uintptr) {
	w.density = w.scale * float64(max(dpi, 96)) / 96
	w.setCell(w.Cell())
	w.recell()
}

// monitor is the monitor most of the window is on
func (w *Window) monitor() monitorInfo {
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	monitor, _, _ := procMonitorFromWindow.Call(w.hwnd, monitorNearest)
	procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	return info
}

// fullscreen makes the window a borderless one over its monitor, the display's
// mode left as it is, or puts back the frame and the place it had before
func (w *Window) fullscreen(on bool) {
	style, _, _ := procGetWindowLongPtrW.Call(w.hwnd, gwlStyle)
	if w.full = on; !on {
		procSetWindowLongPtrW.Call(w.hwnd, gwlStyle, style&^wsPopup|wsOverlappedWindow)
		procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&w.before)))
		procSetWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpFrameChanged)
		return
	}
	w.before.length = uint32(unsafe.Sizeof(w.before))
	procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&w.before)))
	over := w.monitor().monitor
	procSetWindowLongPtrW.Call(w.hwnd, gwlStyle, style&^wsOverlappedWindow|wsPopup)
	procSetWindowPos.Call(w.hwnd, 0, uintptr(over.Left), uintptr(over.Top), uintptr(over.Right-over.Left),
		uintptr(over.Bottom-over.Top), swpNoZOrder|swpFrameChanged)
}

// present draws the newest frame posted
func (w *Window) present() {
	w.rasterize()
	w.draw()
}

// draw shows the raster's frame over its cells, centered, and fills what it leaves
// of the window black: as it is where the raster drew at the cell's size, each of
// its pixels a block where it drew at the strike's
func (w *Window) draw() {
	fw, fh := w.raster.Bounds()
	rw, rh := w.raster.Cell()
	dw, dh := fw/rw*w.cw, fh/rh*w.ch
	x, y := (w.winW-dw)/2, (w.winH-dh)/2
	for _, m := range [4][4]int{{0, 0, w.winW, y}, {0, y + dh, w.winW, w.winH - y - dh},
		{0, y, x, dh}, {x + dw, y, w.winW - x - dw, dh}} {
		if m[2] > 0 && m[3] > 0 {
			procPatBlt.Call(w.dc, uintptr(m[0]), uintptr(m[1]), uintptr(m[2]), uintptr(m[3]), blackness)
		}
	}
	if fw == 0 || fh == 0 {
		return
	}
	// A negative height is a frame whose first row is its top one
	w.info.width, w.info.height, w.info.sizeImage = int32(fw), -int32(fh), uint32(fw*fh*4)
	procStretchDIBits.Call(w.dc, uintptr(x), uintptr(y), uintptr(dw), uintptr(dh), 0, 0, uintptr(fw), uintptr(fh),
		uintptr(unsafe.Pointer(unsafe.SliceData(w.raster.Pix()))), uintptr(unsafe.Pointer(&w.info)), 0, srcCopy)
}

// recell has the raster draw a whole multiple of the strike's cell at the strike's
// size, for GDI to stretch, and any other cell at its own, so that nothing is
// filtered on the way to the screen
func (w *Window) recell() {
	rw, rh := w.atlas.Width, w.atlas.Height
	if w.cw%rw != 0 || w.ch%rh != 0 {
		rw, rh = w.cw, w.ch
	}
	w.raster.SetCell(rw, rh)
}

// wndProc is the window procedure of the class: a message of a window's making
// comes before its handle is known, and is the system's
func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	live.RLock()
	w := live.windows[hwnd]
	live.RUnlock()
	if w == nil {
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	return w.handle(message, wParam, lParam)
}

// handle takes one message; what it leaves is the system's
func (w *Window) handle(message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmPaint:
		// Validated before the frame is taken, so one posted meanwhile is painted too
		procValidateRect.Call(w.hwnd, 0)
		w.present()
		return 0
	case wmEraseBkgnd:
		return 1 // draw leaves no pixel of the window as it was
	case wmSize:
		// Restored or maximized: a minimized window keeps its grid
		if wParam&^sizeMaximized == 0 && w.fit(int(lParam&0xffff), int(lParam>>16&0xffff)) && w.placed {
			w.postGrid()
		}
		return 0
	case wmSizing:
		w.snap(wParam, at[windows.Rect](lParam))
		return 1
	case wmDpiChanged:
		w.rescaled(wParam & 0xffff)
		w.rasterize() // in the strike the new density takes, for the resize to draw
		to := at[windows.Rect](lParam)
		procSetWindowPos.Call(w.hwnd, 0, uintptr(to.Left), uintptr(to.Top), uintptr(to.Right-to.Left),
			uintptr(to.Bottom-to.Top), swpNoZOrder|swpNoActivate)
		if w.regrid() && w.placed { // where the size stayed and the cell did not
			w.postGrid()
		}
		return 0
	case wmClose:
		w.post(terminal.Event{Type: terminal.EventClosed})
		return 0
	case wmStop:
		procPostQuitMessage.Call(0)
		return 0
	case wmKillFocus, wmActivate, wmEnterMenuLoop, wmEnterSizeMove:
		// The keys' releases go elsewhere from here: to another window, or to a loop
		// of the system's that moves the window or runs its menu
		if message != wmActivate || wParam&0xffff == 0 {
			w.keys.Lost()
		}
	case wmKeyDown, wmSysKeyDown:
		if message == wmSysKeyDown && wParam == vkF4 {
			break // Alt+F4 is the system's, which answers with wmClose
		}
		w.keyDown(wParam, lParam)
		return 0
	case wmKeyUp, wmSysKeyUp: // never the system's, which would open the menu on Alt or F10
		w.keys.Up(keyCode(wParam, lParam))
		w.down = noKey
		return 0
	case wmChar, wmSysChar:
		if message == wmSysChar && wParam == ' ' {
			break // Alt+Space opens the system menu
		}
		w.char(message == wmSysChar, rune(uint16(wParam)), lParam)
		return 0
	case wmMouseMove, wmLButtonDown, wmLButtonUp, wmRButtonDown, wmRButtonUp, wmMButtonDown, wmMButtonUp:
		w.mouse(message, wParam, lParam)
		return 0
	case wmXButtonDown, wmXButtonUp:
		w.mouse(message, wParam, lParam)
		return 1
	case wmMouseWheel, wmMouseHWheel:
		w.wheel(message == wmMouseHWheel, wParam)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(w.hwnd, uintptr(message), wParam, lParam)
	return r
}

// snap moves the edge a border is dragged by to the nearest whole cell, as X size
// hints ask of a window manager
func (w *Window) snap(edge uintptr, to *windows.Rect) {
	var outer, client windows.Rect
	procGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	// The frame keeps its size through the drag
	fw, fh := outer.Right-outer.Left-client.Right, outer.Bottom-outer.Top-client.Bottom
	cw, ch := int32(w.cw), int32(w.ch)
	width := fw + max((to.Right-to.Left-fw+cw/2)/cw, 1)*cw
	height := fh + max((to.Bottom-to.Top-fh+ch/2)/ch, 1)*ch
	if edge%3 == 1 { // WMSZ_LEFT, TOPLEFT, BOTTOMLEFT
		to.Left = to.Right - width
	} else {
		to.Right = to.Left + width
	}
	if edge >= 3 && edge <= 5 { // WMSZ_TOP, TOPLEFT, TOPRIGHT
		to.Top = to.Bottom - height
	} else {
		to.Bottom = to.Top + height
	}
}

// mouseButtons are the button messages from WM_LBUTTONDOWN on, each a button going
// down or coming up; an X button's names the back one, its wParam which of the two
var mouseButtons = [...]struct {
	button terminal.MouseButton
	action terminal.MouseAction
}{
	wmLButtonDown - wmLButtonDown: {terminal.MouseBtnLeft, terminal.MouseActionPress},
	wmLButtonUp - wmLButtonDown:   {terminal.MouseBtnLeft, terminal.MouseActionRelease},
	wmRButtonDown - wmLButtonDown: {terminal.MouseBtnRight, terminal.MouseActionPress},
	wmRButtonUp - wmLButtonDown:   {terminal.MouseBtnRight, terminal.MouseActionRelease},
	wmMButtonDown - wmLButtonDown: {terminal.MouseBtnMiddle, terminal.MouseActionPress},
	wmMButtonUp - wmLButtonDown:   {terminal.MouseBtnMiddle, terminal.MouseActionRelease},
	wmXButtonDown - wmLButtonDown: {terminal.MouseBtnBack, terminal.MouseActionPress},
	wmXButtonUp - wmLButtonDown:   {terminal.MouseBtnBack, terminal.MouseActionRelease},
}

// mouse posts a button or a move message as a terminal reports it, a move carrying
// the first button held
func (w *Window) mouse(message uint32, wParam, lParam uintptr) {
	button, action := terminal.MouseBtnNone, terminal.MouseActionMove
	switch {
	case message != wmMouseMove:
		b := mouseButtons[message-wmLButtonDown]
		if button, action = b.button, b.action; button == terminal.MouseBtnBack && wParam>>16&0xffff == 2 {
			button = terminal.MouseBtnForward
		}
		// Captured while a button is held, so a drag is followed past the window's edge
		if action == terminal.MouseActionPress {
			procSetCapture.Call(w.hwnd)
		} else if wParam&mkButtons == 0 {
			procReleaseCapture.Call()
		}
	case wParam&mkLButton != 0:
		button = terminal.MouseBtnLeft
	case wParam&mkMButton != 0:
		button = terminal.MouseBtnMiddle
	case wParam&mkRButton != 0:
		button = terminal.MouseBtnRight
	}
	if ev, ok := w.pointerEvent(int(int16(lParam)), int(int16(lParam>>16)), button, action, held()); ok {
		w.post(ev)
	}
}

// wheel takes wheel movement a notch at a time, a finer wheel's adding up to one:
// a zoom step with Ctrl held, as on a page, or else a wheel button's press
func (w *Window) wheel(sideways bool, wParam uintptr) {
	rest, fore, back := &w.wheelV, terminal.MouseBtnWheelUp, terminal.MouseBtnWheelDown
	if sideways {
		rest, fore, back = &w.wheelH, terminal.MouseBtnWheelRight, terminal.MouseBtnWheelLeft
	}
	*rest += int(int16(wParam >> 16))
	mods := held()
	// The pointer's place on the screen as the message was posted: its own lParam
	// is the screen's too, but the window's in Wine for a sideways wheel
	pos, _, _ := procGetMessagePos.Call()
	point := [2]int32{int32(int16(pos)), int32(int16(pos >> 16))}
	procScreenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&point)))
	for *rest >= wheelDelta || *rest <= -wheelDelta {
		button, by := fore, zoomStep
		if *rest < 0 {
			button, by = back, 1/zoomStep
			*rest += 2 * wheelDelta
		}
		*rest -= wheelDelta
		if !sideways && mods&terminal.ModCtrl != 0 {
			w.zoom(by)
		} else if ev, ok := w.pointerEvent(int(point[0]), int(point[1]), button, terminal.MouseActionPress, mods); ok {
			w.post(ev)
		}
	}
}

// paste posts the clipboard's text as one paste, its line ends made LF
func (w *Window) paste() {
	if ok, _, _ := procOpenClipboard.Call(w.hwnd); ok == 0 {
		return
	}
	var text string
	if data, _, _ := procGetClipboardData.Call(cfUnicodeText); data != 0 {
		if p, _, _ := procGlobalLock.Call(data); p != 0 {
			text = windows.UTF16PtrToString(at[uint16](p))
			procGlobalUnlock.Call(data)
		}
	}
	procCloseClipboard.Call()
	if text != "" {
		w.post(terminal.Event{Type: terminal.EventPaste, Text: strings.ReplaceAll(text, "\r\n", "\n")})
	}
}
