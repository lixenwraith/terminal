package terminal

import (
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/lixenwraith/color"
	"golang.org/x/term"
)

// Attr represents text attributes and how to read a cell's colors (bitmask)
type Attr uint16

const (
	AttrNone      Attr = 0
	AttrBold      Attr = 1 << 0
	AttrDim       Attr = 1 << 1
	AttrItalic    Attr = 1 << 2
	AttrUnderline Attr = 1 << 3
	AttrBlink     Attr = 1 << 4
	AttrReverse   Attr = 1 << 5
	AttrFg256     Attr = 1 << 6 // Fg.R is 256-color palette index
	AttrBg256     Attr = 1 << 7 // Bg.R is 256-color palette index
	AttrFgDefault Attr = 1 << 8 // Fg is the terminal's own (SGR 39); Fg is ignored
	AttrBgDefault Attr = 1 << 9 // Bg is the terminal's own (SGR 49); Bg is ignored
)

// AttrStyle masks only the style bits (excludes color mode flags)
const AttrStyle Attr = AttrBold | AttrDim | AttrItalic | AttrUnderline | AttrBlink | AttrReverse

// AttrFgColor and AttrBgColor mask the bits that say how to read Fg and Bg
const (
	AttrFgColor Attr = AttrFg256 | AttrFgDefault
	AttrBgColor Attr = AttrBg256 | AttrBgDefault
)

// Cell represents a single terminal cell
type Cell struct {
	Rune  rune
	Fg    color.RGB
	Bg    color.RGB
	Attrs Attr
}

// Terminal provides low-level terminal access
type Terminal interface {
	// Init enters raw mode, alternate screen buffer, hides cursor
	Init() error

	// Fini restores terminal state. Safe to call multiple times
	Fini()

	// Size returns current terminal dimensions
	Size() (width, height int)

	// ResizeChan returns channel that receives resize events
	ResizeChan() <-chan ResizeEvent

	// ColorMode returns detected color capability
	ColorMode() ColorMode

	// Flush writes cell buffer to terminal
	// Cells are row-major: cells[y*width + x]
	Flush(cells []Cell, width, height int)

	// Clear fills screen with specified background color
	Clear(bg color.RGB)

	// SetCursorVisible shows/hides cursor
	SetCursorVisible(visible bool)

	// MoveCursor positions cursor (0-indexed)
	MoveCursor(x, y int)

	// Sync forces full redraw
	Sync()

	// PollEvent blocks until next input event
	PollEvent() Event

	// PostEvent injects a synthetic event
	PostEvent(Event)

	// SetMouseMode enables/disables mouse event reporting
	// Modes can be combined: MouseModeClick | MouseModeDrag
	SetMouseMode(mode MouseMode) error

	// SetPasteMode turns bracketed paste on or off: on, a paste arrives as
	// one EventPaste rather than as keys, so its newlines don't press Enter
	SetPasteMode(on bool) error
}

// ResizeEvent represents a terminal resize
type ResizeEvent struct {
	Width  int
	Height int
}

// termImpl implements Terminal using the Backend interface
type termImpl struct {
	backend Backend

	output      *outputBuffer
	input       *inputReader
	resizeCh    chan ResizeEvent
	syntheticCh chan Event

	cursorVisible atomic.Bool

	mu          sync.Mutex
	initialized bool
	finalized   bool
	mouseMode   MouseMode
	paste       bool
}

// New creates a new Terminal instance. Without a mode it uses
// DetectColorMode's, or ColorModeNone when NO_COLOR is set; an explicit mode
// overrides both, as a --color=always or never would
func New(colorMode ...ColorMode) Terminal {
	b := newBackend()

	c := DetectColorMode()
	if len(colorMode) > 0 {
		c = colorMode[0]
	} else if os.Getenv("NO_COLOR") != "" {
		c = ColorModeNone
	}

	t := &termImpl{
		backend:     b,
		syntheticCh: make(chan Event, 16),
		resizeCh:    make(chan ResizeEvent, 1),
	}

	// Initialize output buffer with backend
	t.output = newOutputBuffer(b, c)
	return t
}

// Init enters raw mode and sets up terminal
func (t *termImpl) Init() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.initialized {
		return nil
	}

	// Initialize backend (raw mode)
	if err := t.backend.Init(); err != nil {
		return err
	}

	w, h := t.backend.Size()
	t.output.resize(w, h)

	// Create input reader wrapping backend
	t.input = newInputReader(t.backend)

	// Set resize handler on backend
	t.backend.SetResizeHandler(func(w, h int) {
		// Non-blocking send to avoid backend blocking
		select {
		case t.resizeCh <- ResizeEvent{Width: w, Height: h}:
		default:
			// Drain and replace to ensure latest size is pending
			select {
			case <-t.resizeCh:
			default:
			}
			select {
			case t.resizeCh <- ResizeEvent{Width: w, Height: h}:
			default:
			}
		}
	})

	// Enter alternate screen, hide cursor
	t.writeRaw(csiAltScreenEnter)
	t.writeRaw(csiCursorHide)

	// DISABLE AUTO-WRAP
	// Prevents terminal scroll/wrap on bottom-right corner write
	t.writeRaw(csiAutoWrapOff)

	// Invisible cursor
	t.cursorVisible.Store(false)

	// Build the LUT at startup, not on the first frame under t.mu
	if t.output.colorMode == ColorMode256 {
		WarmPalette256()
	}

	// Clear screen
	t.output.clear(color.RGB{}, AttrBgDefault)

	// Start input reader
	t.input.start()

	t.initialized = true
	return nil
}

// Fini restores terminal state
func (t *termImpl) Fini() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	// Disable mouse before other cleanup
	if t.mouseMode != MouseModeNone {
		w := t.output.writer
		w.Write(csiMouseMotionOff)
		w.Write(csiMouseDragOff)
		w.Write(csiMouseClickOff)
		w.Write(csiMouseSGROff)
		w.Flush()
	}

	if t.paste {
		t.writeRaw(csiPasteOff)
	}

	// Stop handlers
	if t.input != nil {
		t.input.stop()
	}

	// Show cursor
	t.writeRaw(csiCursorShow)

	// Exit alternate screen
	t.writeRaw(csiAltScreenExit)

	// Re-enable Auto-Wrap AFTER exiting alt screen to ensure the main buffer has wrap enabled
	t.writeRaw(csiAutoWrapOn)

	// Reset attributes
	t.writeRaw(csiSGR0)

	// Backend cleanup
	t.backend.Fini()

	t.finalized = true
}

// Size returns current terminal dimensions
func (t *termImpl) Size() (int, int) {
	return t.backend.Size()
}

// ResizeChan returns the resize event channel
func (t *termImpl) ResizeChan() <-chan ResizeEvent {
	return t.resizeCh
}

// ColorMode returns detected color capability
func (t *termImpl) ColorMode() ColorMode {
	return t.output.colorMode
}

// Flush writes cell buffer to terminal
// Holds lock for entire operation to prevent race with Clear/MoveCursor
func (t *termImpl) Flush(cells []Cell, width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	// Validation against backend size; if mismatch, drop frame to prevent resize race corruption
	currW, currH := t.backend.Size()
	if currW != width || currH != height {
		return
	}

	t.output.flush(cells, width, height)
}

// Clear fills screen with background color
func (t *termImpl) Clear(bg color.RGB) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	t.output.clear(bg, 0)
}

// SetCursorVisible shows/hides cursor
func (t *termImpl) SetCursorVisible(visible bool) {
	if t.cursorVisible.Swap(visible) == visible {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	w := t.output.writer
	if visible {
		w.Write(csiCursorShow)
	} else {
		w.Write(csiCursorHide)
	}
	w.Flush()
}

// MoveCursor positions cursor (0-indexed)
func (t *termImpl) MoveCursor(x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	if t.output != nil {
		t.output.invalidateCursor()
	}

	w, h := t.backend.Size()
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= w {
		x = w - 1
	}
	if y >= h {
		y = h - 1
	}

	// Write through buffered writer to maintain stream order
	wBuf := t.output.writer
	writeCursorPos(wBuf, x, y)
	wBuf.Flush()
}

// Sync forces full redraw
func (t *termImpl) Sync() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	// Clear terminal before full redraw
	// Diff-based rendering assumes physical terminal matches front buffer state
	t.output.clear(color.RGB{}, AttrBgDefault)
	t.output.forceFullRedraw()
}

// PollEvent blocks until next input event
func (t *termImpl) PollEvent() Event {
	// Check synthetic events first
	select {
	case ev := <-t.syntheticCh:
		return ev
	default:
	}

	// Wait for input or resize
	select {
	case ev := <-t.syntheticCh:
		return ev
	case ev := <-t.input.events():
		return ev
	case re := <-t.resizeCh:
		// We can return resize event directly
		return Event{
			Type:   EventResize,
			Width:  re.Width,
			Height: re.Height,
		}
	}
}

// PostEvent injects a synthetic event
func (t *termImpl) PostEvent(ev Event) {
	select {
	case t.syntheticCh <- ev:
	default:
		// Channel full, drop
	}
}

// SetMouseMode enables or disables mouse mode
func (t *termImpl) SetMouseMode(mode MouseMode) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return nil
	}

	oldMode := t.mouseMode
	t.mouseMode = mode

	w := t.output.writer

	// Disable modes no longer needed (reverse order of enable)
	if oldMode&MouseModeMotion != 0 && mode&MouseModeMotion == 0 {
		w.Write(csiMouseMotionOff)
	}
	if oldMode&MouseModeDrag != 0 && mode&MouseModeDrag == 0 {
		w.Write(csiMouseDragOff)
	}
	if oldMode&MouseModeClick != 0 && mode&MouseModeClick == 0 {
		w.Write(csiMouseClickOff)
	}

	// Disable SGR if disabling all mouse
	if mode == MouseModeNone && oldMode != MouseModeNone {
		w.Write(csiMouseSGROff)
	}

	// Enable SGR first if enabling any mouse mode
	if mode != MouseModeNone && oldMode == MouseModeNone {
		w.Write(csiMouseSGROn)
	}

	// Enable new modes (click is base, drag extends, motion extends further)
	if mode&MouseModeClick != 0 && oldMode&MouseModeClick == 0 {
		w.Write(csiMouseClickOn)
	}
	if mode&MouseModeDrag != 0 && oldMode&MouseModeDrag == 0 {
		w.Write(csiMouseDragOn)
	}
	if mode&MouseModeMotion != 0 && oldMode&MouseModeMotion == 0 {
		w.Write(csiMouseMotionOn)
	}

	w.Flush()
	return nil
}

// SetPasteMode turns bracketed paste on or off
func (t *termImpl) SetPasteMode(on bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized || t.paste == on {
		return nil
	}
	t.paste = on
	t.input.pasteMode.Store(on)
	w := t.output.writer
	if on {
		w.Write(csiPasteOn)
	} else {
		w.Write(csiPasteOff)
	}
	return w.Flush()
}

// writeRaw writes raw bytes to output
func (t *termImpl) writeRaw(data []byte) {
	t.backend.Write(data)
}

// EmergencyReset attempts to restore terminal to sane state
// Call this from panic recovery if Fini() cannot be called normally. Given a
// redirected stdout, it writes to the controlling terminal instead.
func EmergencyReset(w io.Writer) {
	if f, ok := w.(*os.File); ok && !term.IsTerminal(int(f.Fd())) {
		if tty := controllingTTY(); tty != nil {
			defer tty.Close()
			w = tty
		}
	}

	// Disable mouse tracking
	w.Write(csiMouseMotionOff)
	w.Write(csiMouseDragOff)
	w.Write(csiMouseClickOff)
	w.Write(csiMouseSGROff)
	w.Write(csiPasteOff)

	// Write sequences to provided writer
	w.Write(csiCursorShow)
	w.Write(csiAltScreenExit)
	w.Write(csiSGR0)
	w.Write(csiAutoWrapOn)
	w.Write(csiRIS)

	// Flush if it's a file
	if f, ok := w.(*os.File); ok {
		f.Sync()
	}

	// Attempt raw mode reset via stty - escape sequences alone don't restore termios
	// This is best-effort; ignore errors in crash context
	resetTerminalMode()
}
