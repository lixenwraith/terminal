//go:build linux || freebsd || windows

// Package window is a desktop window as a terminal.Host, drawn by glyph.Raster. On
// X11, under Linux and FreeBSD, Xlib opens it and EGL shows each frame as one
// texture, through GLES 2 or desktop OpenGL, all loaded at run time with purego; on
// Windows it is a Win32 window and GDI shows the frame. Neither needs cgo. Ctrl
// with =, - or the wheel zooms within Config.Zoom; Ctrl with 0 undoes the zoom.
package window

import (
	"cmp"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/glyph"
)

// Config is the window as it opens: its title, the font cells are drawn with, the
// grid it is sized for and the window pixels a pixel of the font's largest strike
// covers each way, at least 1, which Windows multiplies by its display's scale
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
	// Maximized or Fullscreen is the state the window opens in, on X11 asked of the
	// window manager; the grid follows the size that gives, as it follows any resize.
	// Windows has no manager to toggle full screen: F11 and Alt+Enter do there.
	Maximized, Fullscreen bool
}

// Window is a terminal.Host over a window of the platform's. Frames, Sync and the
// pointer mode may be set from any goroutine; one locked OS thread owns the window
// and what draws in it. Its close button posts terminal.EventClosed and leaves it
// open: Close, which the Fini of NewWithHost's Terminal calls, closes it. On
// Windows Ctrl-Shift-V and Shift-Insert post the clipboard's text as a paste.
type Window struct {
	font    *glyph.Font
	own     float64 // the font's own cell height, its largest strike's
	density float64 // the window's pixels to a dp
	keeps   glyph.Zoom
	events  chan terminal.Event
	ready   chan struct{} // a frame is waiting to be drawn
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once

	grid    atomic.Uint64 // cols<<32 | rows
	dp      atomic.Uint64 // the zoom, a cell's height in dp as float64 bits, 0 for the font's own
	pointer atomic.Uint32 // pointerOff, pointerButtons or pointerMotion
	asks    chan keyAsk   // the latest SetKeyReport the window has not taken
	keys    terminal.HostKeys

	mu             sync.Mutex
	frame          []terminal.Cell
	frameW, frameH int
	posted         time.Time // when the waiting frame was posted, zero once it is taken
	sync           bool

	// Owned by the window's thread
	native
	atlas      *glyph.Atlas // the strike the zoom's cell is drawn with
	raster     *glyph.Raster
	cells      []terminal.Cell
	winW, winH int
	cw, ch     int // a cell's window pixels, at the density and the zoom
	lastCell   [2]int
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

// eventQueue matches the terminal's
const eventQueue = 256

// zoomStep is the factor one press or wheel step zooms the cell by, as on a page
const zoomStep = 1.1

// Open opens the window and starts its thread; it returns once the window is
// placed on screen, so Size is the grid it then holds
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
		font:    cfg.Font,
		own:     float64(strikes[len(strikes)-1].Height),
		density: float64(max(cfg.Scale, 1)),
		keeps:   cfg.Zoom,
		events:  make(chan terminal.Event, eventQueue),
		ready:   make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		asks:    make(chan keyAsk, 1),
	}
	w.keys.Post = func(ev terminal.Event) { w.post(ev) }
	w.setCell(cfg.Cell)
	w.setGrid(cfg.Cols, cfg.Rows)
	return w
}

// Close stops the thread and releases the window and what draws in it
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

// SetKeyReport asks for each key's release, which the window answers with an
// EventKeyReport: presses alone on an X server that repeats a held key as releases
// and presses, since those can't be told from the player's own
func (w *Window) SetKeyReport(report terminal.KeyReport, allow terminal.KeySource) error {
	for a := (keyAsk{report, allow}); ; {
		select {
		case w.asks <- a:
			return nil
		default:
		}
		select { // an ask the window has not taken gives way to this one
		case <-w.asks:
		default:
		}
	}
}

func (w *Window) setGrid(cols, rows int) { w.grid.Store(uint64(cols)<<32 | uint64(uint32(rows))) }

// Cell is the zoom the player left the window at, a cell's height in dp, 0 for the
// font's own, which a program keeps for the next run
func (w *Window) Cell() float64 { return math.Float64frombits(w.dp.Load()) }

// setCell sizes a cell by the density and the zoom dp, 0 for the font's own, and
// takes the strike that draws it, a raster of its own with it
func (w *Window) setCell(dp float64) {
	w.dp.Store(math.Float64bits(dp))
	strike, cw, ch := w.font.Cell(w.density, cmp.Or(dp, w.own))
	if w.cw, w.ch = cw, ch; strike != w.atlas {
		w.atlas, w.raster = strike, glyph.NewRaster(strike)
	}
}

// cell is a cell's size in window pixels
func (w *Window) cell() (int, int) { return w.cw, w.ch }

// rasterize takes the newest frame posted and draws its changed cells in the raster
func (w *Window) rasterize() {
	w.mu.Lock()
	w.cells = append(w.cells[:0], w.frame...)
	width, height, sync := w.frameW, w.frameH, w.sync
	w.posted, w.sync = time.Time{}, false
	w.mu.Unlock()
	if sync {
		w.raster.Sync()
	}
	w.raster.Flush(w.cells, width, height)
}

// zoom takes a zoom step: the grid the window's pixels hold follows the cell and is
// posted as a resize, as a resized window's is. The X size hints stay the opening
// cell's: new ones would have the manager resize the window.
func (w *Window) zoom(by float64) bool {
	if !w.rescale(by) {
		return true
	}
	w.recell()
	w.present() // the newest frame again, which another strike may now draw
	return !w.regrid() || w.postGrid()
}

// rescale sets the cell a zoom step leaves, by scaling it within the range the
// window keeps to and 0 putting back the font's own; false where it stays as it is
func (w *Window) rescale(by float64) bool {
	kept, dp := w.Cell(), 0.0
	if was := cmp.Or(kept, w.own); by != 0 {
		if dp = w.keeps.Scale(w.font, w.density, was, by); dp == was {
			return false
		}
	}
	if dp == kept {
		return false
	}
	w.setCell(dp)
	return true
}

// fit records the window's pixel size and reports whether its grid changed
func (w *Window) fit(width, height int) bool {
	if width == w.winW && height == w.winH {
		return false
	}
	w.winW, w.winH = width, height
	w.draw()
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

// keyEvent is the terminal event for a key press: key for a non-printing key, else
// r, the character the layout gives the key with Shift applied. ok is false for a
// key a terminal never sees: a modifier alone, a dead key, or Ctrl with a character
// that sends no control byte.
func keyEvent(key terminal.Key, r rune, mods terminal.Modifier) (terminal.Event, bool) {
	ev := terminal.Event{Type: terminal.EventKey, Key: key}
	if key != terminal.KeyNone {
		if key == terminal.KeyBacktab {
			ev.Modifiers = terminal.ModShift
		}
		// A terminal reports modifiers on the editing, navigation and function keys only
		if key == terminal.KeyDelete || key >= terminal.KeyUp && key <= terminal.KeyF12 {
			ev.Modifiers = mods
		}
		return ev, true
	}
	if r == 0 {
		return ev, false
	}
	ev.Key, ev.Rune, ev.Modifiers = terminal.KeyRune, r, mods&terminal.ModAlt
	if mods&terminal.ModCtrl != 0 {
		var ok bool
		if ev.Key, ok = terminal.ControlKey(r); !ok {
			return ev, false
		}
		ev.Rune = 0
	}
	return ev, true
}

// pointerEvent is the terminal event for the pointer at a window pixel, as SGR mouse
// reporting gives it: button pressed or released, or for a move the button held,
// none for a bare one. A move is reported once per cell, as a drag with a button
// and without one only where every motion is asked for.
func (w *Window) pointerEvent(px, py int, button terminal.MouseButton, action terminal.MouseAction, mods terminal.Modifier) (terminal.Event, bool) {
	mode := w.pointer.Load()
	if mode == pointerOff {
		return terminal.Event{}, false
	}
	m := terminal.Event{Type: terminal.EventMouse, Modifiers: mods, MouseBtn: button, MouseAction: action}
	m.MouseX, m.MouseY = w.cellAt(px, py)
	moved := [2]int{m.MouseX, m.MouseY} != w.lastCell
	w.lastCell = [2]int{m.MouseX, m.MouseY}
	if action != terminal.MouseActionMove {
		return m, true
	}
	if button != terminal.MouseBtnNone {
		m.MouseAction = terminal.MouseActionDrag
	}
	return m, moved && (button != terminal.MouseBtnNone || mode == pointerMotion)
}

// cellAt is the cell under a window pixel, the frame being drawn centered; a pixel
// in the margin takes the nearest edge cell
func (w *Window) cellAt(px, py int) (int, int) {
	cols, rows := w.Size()
	if fw, fh := w.raster.Bounds(); fw > 0 {
		rw, rh := w.raster.Cell()
		cols, rows = fw/rw, fh/rh
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
