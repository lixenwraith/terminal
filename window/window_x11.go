//go:build linux || freebsd

package window

import (
	"errors"
	"runtime"
	"time"
	"unsafe"

	"github.com/lixenwraith/terminal"
	"golang.org/x/sys/unix"
)

// native is the X window on its display and the context that draws in it
type native struct {
	dpy, win   uintptr
	deleteAtom uintptr
	gpu        *gpu
	detectable bool // the server repeats a held key as presses alone
}

// pollWake bounds how long the X watcher takes to notice Close
const pollWake = 100 // ms

// settleWait bounds Open's wait for the window manager to place the window, as the
// page's fit is bounded; settleQuiet is how long the size must then hold, since a
// manager may size a window just after mapping it, as a Wayland compositor's does
const (
	settleWait  = time.Second
	settleQuiet = 100 * time.Millisecond
)

// X event types and the input mask that selects them
const (
	xKeyPress        = 2
	xKeyRelease      = 3
	xButtonPress     = 4
	xButtonRelease   = 5
	xMotionNotify    = 6
	xFocusOut        = 10
	xExpose          = 12
	xMapNotify       = 19
	xConfigureNotify = 22
	xClientMessage   = 33
	xEventMask       = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<6 | 1<<15 | 1<<17 | 1<<21
	xaAtom           = 4 // the ATOM property type
	propModeReplace  = 0
	notifyInferior   = 2 // focus event details
	notifyPointer    = 5
)

// xEvent is Xlib's XEvent union, 24 longs; fields are read at their LP64 offsets
type xEvent [24]int64

func (e *xEvent) int32At(off uintptr) int32 { return *(*int32)(unsafe.Add(unsafe.Pointer(e), off)) }
func (e *xEvent) uint64At(off uintptr) uint64 {
	return *(*uint64)(unsafe.Add(unsafe.Pointer(e), off))
}

// sizeHints is XSizeHints, so the window manager resizes the window by whole cells
type sizeHints struct {
	flags                  int64
	x, y, width, height    int32
	minW, minH, maxW, maxH int32
	incW, incH             int32
	minAspect, maxAspect   [2]int32
	baseW, baseH           int32
	gravity                int32
	_                      int32
}

const hintMinSize, hintResizeInc, hintBaseSize = 1 << 4, 1 << 6, 1 << 8

// run opens the window on $DISPLAY, reports to opened once the window manager has
// placed it (settle), and is its thread until Close
func (w *Window) run(cfg Config, opened chan<- error) {
	defer close(w.done)
	// Xlib and the GL context belong to the thread that made them
	runtime.LockOSThread()
	if err := w.open(cfg); err != nil {
		opened <- err
		return
	}
	defer w.release()
	w.settle(cfg.Fullscreen || cfg.Maximized)
	opened <- nil

	wake := make(chan struct{})
	go w.watch(int(xConnectionNumber(w.dpy)), wake)
	for {
		if !w.pump() {
			return
		}
		select {
		case <-w.stop:
			return
		case <-wake:
		case <-w.ready:
			w.present()
		case a := <-w.asks:
			w.keys.Ask(a.report, a.allow, w.detectable)
		}
	}
}

func (w *Window) open(cfg Config) error {
	if w.dpy = xOpenDisplay(0); w.dpy == 0 {
		return errors.New("window: cannot open the X display; is DISPLAY set?")
	}
	screen := xDefaultScreen(w.dpy)
	cw, ch := w.cell()
	w.winW, w.winH = cfg.Cols*cw, cfg.Rows*ch
	black := xBlackPixel(w.dpy, screen)
	w.win = xCreateSimpleWindow(w.dpy, xRootWindow(w.dpy, screen), 0, 0, uint32(w.winW), uint32(w.winH), 0, black, black)
	xSelectInput(w.dpy, w.win, xEventMask)
	if xkbSetDetectableAutoRepeat != nil {
		var supported int32
		w.detectable = xkbSetDetectableAutoRepeat(w.dpy, 1, &supported) != 0 && supported != 0
	}
	xStoreName(w.dpy, w.win, cfg.Title)
	w.deleteAtom = xInternAtom(w.dpy, "WM_DELETE_WINDOW", 0)
	xSetWMProtocols(w.dpy, w.win, &w.deleteAtom, 1)
	xSetWMNormalHints(w.dpy, w.win, &sizeHints{flags: hintMinSize | hintResizeInc | hintBaseSize,
		minW: int32(cw), minH: int32(ch), incW: int32(cw), incH: int32(ch)})
	// _NET_WM_STATE set before the map is the state the window manager opens with
	if cfg.Fullscreen || cfg.Maximized {
		states := []uintptr{xInternAtom(w.dpy, "_NET_WM_STATE_FULLSCREEN", 0)}
		if !cfg.Fullscreen {
			states = []uintptr{xInternAtom(w.dpy, "_NET_WM_STATE_MAXIMIZED_VERT", 0),
				xInternAtom(w.dpy, "_NET_WM_STATE_MAXIMIZED_HORZ", 0)}
		}
		xChangeProperty(w.dpy, w.win, xInternAtom(w.dpy, "_NET_WM_STATE", 0), xaAtom, 32, propModeReplace,
			&states[0], int32(len(states)))
	}
	xMapWindow(w.dpy, w.win)
	g, err := newGPU(w.dpy, w.win)
	if err != nil {
		w.release()
		return err
	}
	w.gpu = g
	w.recell()
	return nil
}

// settle takes the size the window manager places the window at, so Size is the
// grid on screen when Open returns and no resize is posted for it: once the window
// is mapped, and resized where a state was asked, its size has held for settleQuiet
func (w *Window) settle(asked bool) {
	fds := []unix.PollFd{{Fd: xConnectionNumber(w.dpy), Events: unix.POLLIN}}
	openW, openH, mapped := w.winW, w.winH, false
	bound := time.Now().Add(settleWait) // a wait on the window manager
	until := bound
	for {
		var ev xEvent
		for xPending(w.dpy) > 0 {
			xNextEvent(w.dpy, &ev)
			switch ev.int32At(0) {
			case xMapNotify:
				mapped = true
			case xConfigureNotify:
				w.fit(int(ev.int32At(56)), int(ev.int32At(60)))
			default:
				w.handle(&ev)
				continue
			}
			until = bound
			if quiet := time.Now().Add(settleQuiet); mapped && (!asked || w.winW != openW || w.winH != openH) && quiet.Before(bound) {
				until = quiet
			}
		}
		left := time.Until(until)
		if left <= 0 {
			return
		}
		if _, err := unix.Poll(fds, int(left/time.Millisecond)+1); err != nil && err != unix.EINTR {
			return
		}
	}
}

func (w *Window) release() {
	if w.gpu != nil {
		w.gpu.release()
		w.gpu = nil
	}
	if w.dpy != 0 {
		xDestroyWindow(w.dpy, w.win)
		xCloseDisplay(w.dpy)
		w.dpy = 0
	}
}

// watch wakes the window's thread when the X connection has input
func (w *Window) watch(fd int, wake chan<- struct{}) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, pollWake)
		select {
		case <-w.stop:
			return
		default:
		}
		if err != nil && err != unix.EINTR {
			return
		}
		if n > 0 {
			select {
			case wake <- struct{}{}:
			case <-w.stop:
				return
			}
		}
	}
}

// present draws the newest frame posted
func (w *Window) present() {
	w.rasterize()
	pw, ph := w.raster.Bounds()
	w.gpu.upload(w.raster.Pix(), pw, ph)
	w.draw()
}

// draw shows the frame last uploaded in the window as it now is
func (w *Window) draw() { w.gpu.draw(w.winW, w.winH) }

// recell hands the context the cell a zoom or the scale left
func (w *Window) recell() { w.gpu.cell(w.cw, w.ch, w.atlas.Width, w.atlas.Height) }

// pump handles every X event queued; false once the window was told to stop
// while posting
func (w *Window) pump() bool {
	var ev xEvent
	for xPending(w.dpy) > 0 {
		xNextEvent(w.dpy, &ev)
		if !w.handle(&ev) {
			return false
		}
	}
	return true
}

func (w *Window) handle(ev *xEvent) bool {
	switch ev.int32At(0) {
	case xKeyPress:
		var keysym uint64
		var buf [32]byte
		xLookupString(ev, &buf[0], int32(len(buf)), &keysym, 0)
		state := uint32(ev.int32At(80))
		if by, ok := zoomKey(keysym, state); ok {
			return w.zoom(by)
		}
		if k, ok := keyEvent(keysymKeys[keysym], keysymRune(keysym), modifiers(state)); ok {
			code := uint32(ev.int32At(84))
			w.keys.Down(code, k, w.keys.Held(code))
		}
	case xKeyRelease:
		w.keys.Up(uint32(ev.int32At(84)))
	case xFocusOut:
		// Focus gone to a child, or the pointer's part in a change to the pointer
		// root, leaves the keys here; any other change takes their releases away
		if detail := ev.int32At(44); detail != notifyInferior && detail != notifyPointer {
			w.keys.Lost()
		}
	case xButtonPress, xButtonRelease, xMotionNotify:
		if ev.int32At(0) == xButtonPress {
			if by, ok := zoomWheel(ev.int32At(84), uint32(ev.int32At(80))); ok {
				return w.zoom(by)
			}
		}
		if m, ok := w.mouseEvent(ev); ok {
			return w.post(m)
		}
	case xExpose:
		if ev.int32At(56) == 0 { // the last of a run
			w.draw()
		}
	case xConfigureNotify:
		return w.resize(int(ev.int32At(56)), int(ev.int32At(60)))
	case xClientMessage:
		if uintptr(ev.uint64At(56)) == w.deleteAtom {
			return w.post(terminal.Event{Type: terminal.EventClosed})
		}
	}
	return true
}

// mouseEvent is the terminal event for a button or motion event, a motion carrying
// the first button its state holds; a wheel step is its press alone
func (w *Window) mouseEvent(ev *xEvent) (terminal.Event, bool) {
	state := uint32(ev.int32At(80))
	px, py, mods := int(ev.int32At(64)), int(ev.int32At(68)), modifiers(state)
	kind, button := ev.int32At(0), ev.int32At(84)
	if kind == xMotionNotify {
		held := terminal.MouseBtnNone
		switch {
		case state&stateButton1 != 0:
			held = terminal.MouseBtnLeft
		case state&stateButton2 != 0:
			held = terminal.MouseBtnMiddle
		case state&stateButton3 != 0:
			held = terminal.MouseBtnRight
		}
		return w.pointerEvent(px, py, held, terminal.MouseActionMove, mods)
	}
	named, action := terminal.MouseBtnNone, terminal.MouseActionPress
	if button >= 1 && int(button) <= len(xButtons) {
		named = xButtons[button-1]
	}
	if kind == xButtonRelease {
		action = terminal.MouseActionRelease
	}
	m, ok := w.pointerEvent(px, py, named, action, mods)
	return m, ok && named != terminal.MouseBtnNone && (kind == xButtonPress || button < 4 || button > 7)
}

// xButtons is X's button numbering from 1: left, middle, right, the wheel up,
// down, left and right, then back and forward
var xButtons = []terminal.MouseButton{terminal.MouseBtnLeft, terminal.MouseBtnMiddle, terminal.MouseBtnRight,
	terminal.MouseBtnWheelUp, terminal.MouseBtnWheelDown, terminal.MouseBtnWheelLeft, terminal.MouseBtnWheelRight,
	terminal.MouseBtnBack, terminal.MouseBtnForward}
