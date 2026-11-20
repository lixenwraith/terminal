//go:build linux || freebsd

// Package window is a desktop window as a terminal.Host, on X11 under Linux and
// FreeBSD: Xlib opens it and reads its input, EGL and GLES 2 show the frames
// glyph.Raster draws, each loaded at run time with purego, so the binary keeps
// CGO_ENABLED=0. Ctrl with =, - or the wheel zooms within Config.Zoom, and Ctrl
// with 0 goes back to the font's own size.
package window

import (
	"cmp"
	"errors"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/glyph"
	"golang.org/x/sys/unix"
)

// Config is the window as it opens: its title, the font cells are drawn with, the
// grid it is sized for and the window pixels a pixel of the font's largest strike
// covers each way, at least 1
type Config struct {
	Title      string
	Font       *glyph.Font
	Cols, Rows int
	Scale      int
	// Cell is the zoom the window opens at, a cell's height in dp of Scale pixels,
	// 0 for the font's own, its largest strike's; Zoom is the range its player's
	// zoom keeps to, the zero Zoom none
	Cell float64
	Zoom glyph.Zoom
	// Maximized or Fullscreen is the state the window opens in, asked of the window
	// manager; the grid follows the size it gives, as it follows any resize
	Maximized, Fullscreen bool
}

// Window is a terminal.Host over an X window. Frames, Sync and the pointer mode
// may be set from any goroutine; one locked OS thread owns the display and the
// context. Its close button posts terminal.EventClosed and leaves it open: Close,
// which the Fini of NewWithHost's Terminal calls, closes it.
type Window struct {
	font   *glyph.Font
	own    float64 // the font's own cell height, its largest strike's
	scale  int
	keeps  glyph.Zoom
	events chan terminal.Event
	ready  chan struct{} // a frame is waiting to be drawn
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once

	grid    atomic.Uint64 // cols<<32 | rows
	dp      atomic.Uint64 // the zoom, a cell's height in dp as float64 bits, 0 for the font's own
	pointer atomic.Uint32 // pointerOff, pointerButtons or pointerMotion
	asks    chan keyAsk   // the latest SetKeyReport the thread has not taken
	keys    terminal.HostKeys

	mu             sync.Mutex
	frame          []terminal.Cell
	frameW, frameH int
	posted         time.Time // when the waiting frame was posted, zero once it is taken
	sync           bool

	// Owned by the window's thread
	dpy, win   uintptr
	deleteAtom uintptr
	gpu        *gpu
	atlas      *glyph.Atlas // the strike the zoom's cell is drawn with
	raster     *glyph.Raster
	cells      []terminal.Cell
	winW, winH int
	cw, ch     int // a cell's window pixels, at the scale and the zoom
	lastCell   [2]int
	detectable bool // the server repeats a held key as presses alone
}

type keyAsk struct {
	report terminal.KeyReport
	allow  terminal.KeySource
}

const (
	pointerOff = iota
	pointerButtons
	pointerMotion
)

// eventQueue matches the terminal's; pollWake bounds how long the X watcher takes
// to notice Close
const (
	eventQueue = 256
	pollWake   = 100 // ms
)

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

// Open loads the libraries, opens the window on $DISPLAY and starts its thread; it
// returns once the window manager has placed the window (settle)
func Open(cfg Config) (*Window, error) {
	if cfg.Font == nil || cfg.Cols < 1 || cfg.Rows < 1 {
		return nil, errors.New("window: a font and a grid of at least one cell are required")
	}
	if err := load(); err != nil {
		return nil, err
	}
	w := newWindow(cfg)
	opened := make(chan error, 1)
	go w.run(cfg, opened)
	if err := <-opened; err != nil {
		return nil, err
	}
	return w, nil
}

// newWindow is the window's state before its thread opens it
func newWindow(cfg Config) *Window {
	strikes := cfg.Font.Strikes()
	w := &Window{
		font:   cfg.Font,
		own:    float64(strikes[len(strikes)-1].Height),
		scale:  max(cfg.Scale, 1),
		keeps:  cfg.Zoom,
		events: make(chan terminal.Event, eventQueue),
		ready:  make(chan struct{}, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		asks:   make(chan keyAsk, 1),
	}
	w.keys.Post = func(ev terminal.Event) { w.post(ev) }
	w.setCell(cfg.Cell)
	w.setGrid(cfg.Cols, cfg.Rows)
	return w
}

// Close stops the thread and releases the context and the window
func (w *Window) Close() {
	w.once.Do(func() { close(w.stop) })
	<-w.done
}

func (w *Window) Events() <-chan terminal.Event { return w.events }

// Size is the grid of whole cells the window holds
func (w *Window) Size() (cols, rows int) {
	g := w.grid.Load()
	return int(g >> 32), int(uint32(g))
}

// ColorMode is truecolor: the raster resolves every cell to RGB
func (w *Window) ColorMode() terminal.ColorMode { return terminal.ColorModeTrueColor }

// Flush posts a frame; the window draws the newest one it holds when it is free
func (w *Window) Flush(cells []terminal.Cell, width, height int) {
	if width < 0 || height < 0 || len(cells) < width*height {
		return
	}
	w.mu.Lock()
	w.frame = append(w.frame[:0], cells[:width*height]...)
	w.frameW, w.frameH = width, height
	if w.posted.IsZero() {
		w.posted = time.Now()
	}
	w.mu.Unlock()
	select {
	case w.ready <- struct{}{}:
	default:
	}
}

// Sync draws the next frame whole
func (w *Window) Sync() {
	w.mu.Lock()
	w.sync = true
	w.mu.Unlock()
}

// Blocked is how long the window has held a frame it has not drawn
func (w *Window) Blocked() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.posted.IsZero() {
		return 0
	}
	return time.Since(w.posted)
}

// SetPointerMode selects the pointer events posted: none, buttons and drags, or
// every motion as well
func (w *Window) SetPointerMode(enabled, motion bool) {
	mode := uint32(pointerOff)
	if enabled {
		mode = pointerButtons
		if motion {
			mode = pointerMotion
		}
	}
	w.pointer.Store(mode)
}

// SetKeyReport asks for each key's release, which the window's thread answers with
// an EventKeyReport: presses alone on a server that repeats a held key as releases
// and presses, since those can't be told from the player's own
func (w *Window) SetKeyReport(report terminal.KeyReport, allow terminal.KeySource) error {
	for a := (keyAsk{report, allow}); ; {
		select {
		case w.asks <- a:
			return nil
		default:
		}
		select { // an ask the thread has not taken gives way to this one
		case <-w.asks:
		default:
		}
	}
}

func (w *Window) setGrid(cols, rows int) { w.grid.Store(uint64(cols)<<32 | uint64(uint32(rows))) }

// Cell is the zoom the player left the window at, a cell's height in dp, 0 for the
// font's own, which a program keeps for the next run
func (w *Window) Cell() float64 { return math.Float64frombits(w.dp.Load()) }

// setCell sizes a cell by the scale and the zoom dp, 0 for the font's own, and
// takes the strike that draws it, a raster of its own with it
func (w *Window) setCell(dp float64) {
	w.dp.Store(math.Float64bits(dp))
	strike, cw, ch := w.font.Cell(float64(w.scale), cmp.Or(dp, w.own))
	if w.cw, w.ch = cw, ch; strike != w.atlas {
		w.atlas, w.raster = strike, glyph.NewRaster(strike)
	}
}

// cell is a cell's size in window pixels
func (w *Window) cell() (int, int) { return w.cw, w.ch }

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
	g.cell(cw, ch, w.atlas.Width, w.atlas.Height)
	return nil
}

// zoom takes a zoom step: the grid the window's pixels hold follows the cell and is
// posted as a resize, as a resized window's is. The size hints stay the opening
// cell's: new ones would have the manager resize the window.
func (w *Window) zoom(by float64) bool {
	if !w.rescale(by) {
		return true
	}
	w.gpu.cell(w.cw, w.ch, w.atlas.Width, w.atlas.Height)
	w.present() // the newest frame again, which another strike may now draw
	return !w.regrid() || w.postGrid()
}

// rescale sets the cell a zoom step leaves, by scaling it within the range the
// window keeps to and 0 putting back the font's own; false where it stays as it is
func (w *Window) rescale(by float64) bool {
	kept, dp := w.Cell(), 0.0
	if was := cmp.Or(kept, w.own); by != 0 {
		if dp = w.keeps.Scale(w.font, float64(w.scale), was, by); dp == was {
			return false
		}
	}
	if dp == kept {
		return false
	}
	w.setCell(dp)
	return true
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
	w.mu.Lock()
	w.cells = append(w.cells[:0], w.frame...)
	width, height, sync := w.frameW, w.frameH, w.sync
	w.posted, w.sync = time.Time{}, false
	w.mu.Unlock()
	if sync {
		w.raster.Sync()
	}
	w.raster.Flush(w.cells, width, height)
	pw, ph := w.raster.Bounds()
	w.gpu.upload(w.raster.Pix(), pw, ph)
	w.gpu.draw(w.winW, w.winH)
}

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
		if by, ok := zoomKey(keysym, uint32(ev.int32At(80))); ok {
			return w.zoom(by)
		}
		if k, ok := keyEvent(keysym, uint32(ev.int32At(80))); ok {
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
			w.gpu.draw(w.winW, w.winH)
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

// fit records the window's pixel size and reports whether its grid changed
func (w *Window) fit(width, height int) bool {
	if width == w.winW && height == w.winH {
		return false
	}
	w.winW, w.winH = width, height
	w.gpu.draw(width, height)
	return w.regrid()
}

// regrid sizes the grid to the whole cells the window's pixels hold and reports
// whether it changed
func (w *Window) regrid() bool {
	cols, rows := max(w.winW/w.cw, 1), max(w.winH/w.ch, 1)
	if c, r := w.Size(); c == cols && r == rows {
		return false
	}
	w.setGrid(cols, rows)
	return true
}

func (w *Window) postGrid() bool {
	cols, rows := w.Size()
	return w.post(terminal.Event{Type: terminal.EventResize, Width: cols, Height: rows})
}

// resize follows the window's pixel size and posts the grid when it changed
func (w *Window) resize(width, height int) bool { return !w.fit(width, height) || w.postGrid() }

// mouseEvent is the terminal event for a button or motion event at the cell under
// the pointer, as SGR mouse reporting gives it; motion is reported once per cell
func (w *Window) mouseEvent(ev *xEvent) (terminal.Event, bool) {
	mode := w.pointer.Load()
	if mode == pointerOff {
		return terminal.Event{}, false
	}
	state := uint32(ev.int32At(80))
	m := terminal.Event{Type: terminal.EventMouse, Modifiers: modifiers(state)}
	m.MouseX, m.MouseY = w.cellAt(int(ev.int32At(64)), int(ev.int32At(68)))
	kind := ev.int32At(0)
	if kind == xMotionNotify {
		if [2]int{m.MouseX, m.MouseY} == w.lastCell {
			return m, false
		}
		w.lastCell = [2]int{m.MouseX, m.MouseY}
		switch {
		case state&stateButton1 != 0:
			m.MouseBtn = terminal.MouseBtnLeft
		case state&stateButton2 != 0:
			m.MouseBtn = terminal.MouseBtnMiddle
		case state&stateButton3 != 0:
			m.MouseBtn = terminal.MouseBtnRight
		case mode != pointerMotion:
			return m, false
		}
		m.MouseAction = terminal.MouseActionMove
		if m.MouseBtn != terminal.MouseBtnNone {
			m.MouseAction = terminal.MouseActionDrag
		}
		return m, true
	}
	w.lastCell = [2]int{m.MouseX, m.MouseY}
	button := ev.int32At(84)
	if button < 1 || int(button) > len(xButtons) {
		return m, false
	}
	m.MouseBtn = xButtons[button-1]
	m.MouseAction = terminal.MouseActionPress
	if kind == xButtonRelease {
		if button >= 4 && button <= 7 {
			return m, false // a wheel step is its press
		}
		m.MouseAction = terminal.MouseActionRelease
	}
	return m, true
}

// xButtons is X's button numbering from 1: left, middle, right, the wheel up,
// down, left and right, then back and forward
var xButtons = []terminal.MouseButton{terminal.MouseBtnLeft, terminal.MouseBtnMiddle, terminal.MouseBtnRight,
	terminal.MouseBtnWheelUp, terminal.MouseBtnWheelDown, terminal.MouseBtnWheelLeft, terminal.MouseBtnWheelRight,
	terminal.MouseBtnBack, terminal.MouseBtnForward}

// cellAt is the cell under a window pixel, the frame being drawn centred; a pixel
// in the margin takes the nearest edge cell
func (w *Window) cellAt(px, py int) (int, int) {
	cols, rows := w.Size()
	if fw, fh := w.raster.Bounds(); fw > 0 {
		cols, rows = fw/w.atlas.Width, fh/w.atlas.Height
	}
	cw, ch := w.cell()
	x, y := (px-(w.winW-cols*cw)/2)/cw, (py-(w.winH-rows*ch)/2)/ch
	return min(max(x, 0), cols-1), min(max(y, 0), rows-1)
}

func (w *Window) post(ev terminal.Event) bool {
	select {
	case w.events <- ev:
		return true
	case <-w.stop:
		return false
	}
}
