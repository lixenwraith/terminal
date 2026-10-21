package terminal

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

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

	// Flush hands a frame to the terminal; cells are row-major: cells[y*width + x].
	// It never waits on the terminal: while the last write is still out, the frame
	// is held and drawn once that write returns, a newer one replacing it
	Flush(cells []Cell, width, height int)

	// Blocked reports how long the terminal has held the write in flight, zero
	// while output drains. A terminal that stops reading (a hidden tab, a client
	// that went away, flow control) shows here instead of as a caller that waits
	Blocked() time.Duration

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

	// SetKeyReport asks for keys to be reported at level report. Releases come
	// through the first source in allow the terminal has, the console before
	// the protocol; KeyReportPresses gives up the source in use. It returns at
	// once, and an EventKeyReport follows with what the terminal reports now,
	// so a game learns the outcome of every ask; asks the reader has not yet
	// acted on are answered together, by the latest. A change of source ends
	// the holds it leaves with Lost releases. Until an ask for releases, nothing
	// is sent to the terminal or switched.
	SetKeyReport(report KeyReport, allow KeySource) error
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
	keyProto    bool // the key protocol's flags are pushed
	consoleRaw  bool // the console's keyboard is raw: mouse reports wait

	// Output reaches the terminal on writeLoop's goroutine, so a terminal that
	// stops reading holds that goroutine rather than whoever draws. queued is what
	// was rendered and not yet taken; a frame Flush is handed while a write is out
	// waits in held, newest first, until it returns. Both are guarded by mu.
	queued     []byte
	writing    []byte // owned by writeLoop
	held       []Cell
	heldW      int
	heldH      int
	holding    bool
	writeSince atomic.Int64  // unix nanoseconds the write out began; zero when none is
	drained    chan struct{} // a drain left nothing queued
	wake       chan struct{}
	quit       chan struct{}
	writerDone chan struct{}
}

// finiDrainTimeout bounds how long Fini waits for the terminal to take what is
// queued, the restoring sequences last: a terminal that stopped reading would
// otherwise hold the process at exit
const finiDrainTimeout = 2 * time.Second

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

	return NewWithBackend(b, c)
}

// NewWithBackend creates a Terminal over b in color mode c, for a pipe, an
// in-process emulator or a test standing in for the tty. b must not be nil;
// the Terminal calls its Init on Init and its Fini on Fini, and writes to it
// on its own goroutine as New's does
func NewWithBackend(b Backend, c ColorMode) Terminal {
	return newTerm(b, c)
}

func newTerm(b Backend, c ColorMode) *termImpl {
	return &termImpl{
		backend:     b,
		output:      newOutputBuffer(c),
		syntheticCh: make(chan Event, 16),
		resizeCh:    make(chan ResizeEvent, 1),
		wake:        make(chan struct{}, 1),
		drained:     make(chan struct{}, 1),
		quit:        make(chan struct{}),
		writerDone:  make(chan struct{}),
	}
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
	t.input.sources = backendKeySources(t.backend)
	t.input.write = t.keyWrite
	if t.input.console = backendConsole(t.backend); t.input.console != nil {
		t.input.keysRaw, t.input.keysCooked = t.keysRaw, t.keysCooked
	}

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

	go t.writeLoop()

	// Enter alternate screen, hide cursor
	out := &t.output.buf
	out.Write(csiAltScreenEnter)
	out.Write(csiCursorHide)

	// DISABLE AUTO-WRAP
	// Prevents terminal scroll/wrap on bottom-right corner write
	out.Write(csiAutoWrapOff)

	// Invisible cursor
	t.cursorVisible.Store(false)

	// Build the LUT at startup, not on the first frame under t.mu
	if t.output.colorMode == ColorMode256 {
		WarmPalette256()
	}

	// Clear screen
	t.output.clear(color.RGB{}, AttrBgDefault)
	t.sendLocked()

	// Start input reader
	t.input.start()

	t.initialized = true
	return nil
}

// Fini restores terminal state
func (t *termImpl) Fini() {
	t.mu.Lock()
	if !t.initialized || t.finalized {
		t.mu.Unlock()
		return
	}
	t.finalized = true
	t.holding = false // a frame not yet drawn is not drawn over the restored screen
	w := &t.output.buf

	// Disable mouse before other cleanup
	if t.mouseMode != MouseModeNone {
		w.Write(csiMouseMotionOff)
		w.Write(csiMouseDragOff)
		w.Write(csiMouseClickOff)
		w.Write(csiMouseSGROff)
		t.sendLocked()
	}

	if t.paste {
		w.Write(csiPasteOff)
	}

	// Key reports as the terminal had them, before the screen they were pushed on goes
	if t.keyProto {
		w.Write(csiKeyPop)
		w.Write(csiFocusOff)
		t.keyProto = false
		keyProtoPushed.Store(false)
	}
	t.mu.Unlock()

	// Stop handlers; the reader may be waiting on mu to write, and finds Fini begun
	if t.input != nil {
		t.input.stop()
	}
	t.mu.Lock()

	// Show cursor
	w.Write(csiCursorShow)

	// Exit alternate screen
	w.Write(csiAltScreenExit)

	// Re-enable Auto-Wrap AFTER exiting alt screen to ensure the main buffer has wrap enabled
	w.Write(csiAutoWrapOn)

	// Reset attributes
	w.Write(csiSGR0)
	t.sendLocked()
	t.mu.Unlock()

	// The restoring sequences leave before raw mode does, unless the terminal has
	// stopped reading: then the termios is restored without them
	close(t.quit)
	select {
	case <-t.writerDone:
	case <-time.After(finiDrainTimeout):
	}

	// Backend cleanup
	t.backend.Fini()
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

// Flush renders cells and queues them for the terminal, or holds them while a write
// is out. Holds lock for entire operation to prevent race with Clear/MoveCursor
func (t *termImpl) Flush(cells []Cell, width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	if t.writeSince.Load() != 0 {
		t.held = append(t.held[:0], cells[:min(len(cells), width*height)]...)
		t.heldW, t.heldH, t.holding = width, height, true
		return
	}
	// A frame is a whole screen, so one rendered now supersedes any held
	t.holding = false
	t.renderLocked(cells, width, height)
}

// renderLocked diffs one frame into the queue. Caller MUST hold mu.
func (t *termImpl) renderLocked(cells []Cell, width, height int) {
	// Validation against backend size; if mismatch, drop frame to prevent resize race corruption
	currW, currH := t.backend.Size()
	if currW != width || currH != height {
		return
	}

	t.output.flush(cells, width, height)
	t.sendLocked()
}

// releaseLocked renders a held frame ahead of a control sequence, so the terminal
// receives the two in the order they were asked for. Caller MUST hold mu.
func (t *termImpl) releaseLocked() {
	if t.holding {
		t.holding = false
		t.renderLocked(t.held, t.heldW, t.heldH)
	}
}

// Blocked reports how long the write in flight has waited on the terminal
func (t *termImpl) Blocked() time.Duration {
	if since := t.writeSince.Load(); since != 0 {
		return time.Since(time.Unix(0, since))
	}
	return 0
}

// Clear fills screen with background color
func (t *termImpl) Clear(bg color.RGB) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	t.releaseLocked()
	t.output.clear(bg, 0)
	t.sendLocked()
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

	t.releaseLocked()
	w := &t.output.buf
	if visible {
		w.Write(csiCursorShow)
	} else {
		w.Write(csiCursorHide)
	}
	t.sendLocked()
}

// MoveCursor positions cursor (0-indexed)
func (t *termImpl) MoveCursor(x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || t.finalized {
		return
	}

	t.releaseLocked()
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

	// Through the queue, to keep the stream's order
	writeCursorPos(&t.output.buf, x, y)
	t.sendLocked()
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
	t.releaseLocked()
	t.output.clear(color.RGB{}, AttrBgDefault)
	t.output.forceFullRedraw()
	t.sendLocked()
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
	if t.consoleRaw {
		t.mouseMode = mode // reported once the console's keyboard is cooked again
		return nil
	}
	t.setMouseModeLocked(mode)
	return nil
}

// setMouseModeLocked writes what moves the terminal's mouse reporting to mode.
// Caller MUST hold mu.
func (t *termImpl) setMouseModeLocked(mode MouseMode) {
	oldMode := t.mouseMode
	t.mouseMode = mode

	t.releaseLocked()
	w := &t.output.buf

	// Disable modes no longer needed (reverse order of enable): VTE 0.54+,
	// WezTerm and Windows Terminal keep each one set and report by the highest
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

	// xterm, xterm.js, kitty, tmux and Konsole keep one mode, which setting
	// another replaces and resetting any turns off, and alacritty's and urxvt's
	// set clears the others: on any change the highest mode wanted is written
	// last, even when already on
	var top MouseMode
	switch {
	case mode == oldMode:
	case mode&MouseModeMotion != 0:
		top = MouseModeMotion
	case mode&MouseModeDrag != 0:
		top = MouseModeDrag
	case mode&MouseModeClick != 0:
		top = MouseModeClick
	}

	// Enable new modes, and the highest again (click is base, drag extends,
	// motion extends further)
	if mode&MouseModeClick != 0 && (oldMode&MouseModeClick == 0 || top == MouseModeClick) {
		w.Write(csiMouseClickOn)
	}
	if mode&MouseModeDrag != 0 && (oldMode&MouseModeDrag == 0 || top == MouseModeDrag) {
		w.Write(csiMouseDragOn)
	}
	if mode&MouseModeMotion != 0 && (oldMode&MouseModeMotion == 0 || top == MouseModeMotion) {
		w.Write(csiMouseMotionOn)
	}

	t.sendLocked()
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
	t.releaseLocked()
	w := &t.output.buf
	if on {
		w.Write(csiPasteOn)
	} else {
		w.Write(csiPasteOff)
	}
	t.sendLocked()
	return nil
}

// keyProtoPushed is whether a Terminal has the key protocol's flags pushed, for
// EmergencyReset to pop
var keyProtoPushed atomic.Bool

// keyWrite queues seq, the input reader's request, for the terminal unless Fini
// has begun; pushed is whether the protocol's flags are pushed once it is
// written, which Fini and EmergencyReset undo
func (t *termImpl) keyWrite(seq []byte, pushed bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.initialized || t.finalized {
		return false
	}
	t.releaseLocked()
	t.output.buf.Write(seq)
	t.sendLocked()
	// A terminal that takes a request for its own (an old st restores the
	// cursor on CSI ? u) has moved it as far as the next frame knows
	t.output.invalidateCursor()
	t.keyProto = pushed
	keyProtoPushed.Store(pushed)
	return true
}

// consoleDrainTimeout bounds how long switching a console's keyboard raw waits
// for the console to take what is queued, its mouse reports off among it
const consoleDrainTimeout = 500 * time.Millisecond

// keysRaw switches the console's keyboard raw for the input reader: false
// when it stays cooked, and whether it may not for a later ask. The console
// types its mouse reports into the input the keycodes come through, so they
// are off while it is raw, the console having taken that before the switch,
// which drops reports already sent.
func (t *termImpl) keysRaw() (ok, again bool) {
	c := t.input.console
	if !c.prepare() {
		return false, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.initialized || t.finalized {
		return false, false
	}
	mode := t.mouseMode
	if mode != MouseModeNone {
		t.setMouseModeLocked(MouseModeNone)
		t.mouseMode = mode
	}
	t.consoleRaw = true
	if t.idleLocked(consoleDrainTimeout) && !t.finalized && c.raw() {
		return true, false
	}
	t.mouseBackLocked()
	return false, !t.finalized
}

// keysCooked switches the console's keyboard back for the input reader, and
// mouse reports on again
func (t *termImpl) keysCooked() {
	t.input.console.cooked()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.initialized {
		t.mouseBackLocked()
	}
}

// mouseBackLocked ends the hold on mouse reports the console's keyboard put
// while raw, reporting as asked. Caller MUST hold mu.
func (t *termImpl) mouseBackLocked() {
	t.consoleRaw = false
	if mode := t.mouseMode; mode != MouseModeNone && !t.finalized {
		t.mouseMode = MouseModeNone
		t.setMouseModeLocked(mode)
	}
}

// idleLocked waits, mu released meanwhile, until the terminal has taken all
// that is queued, or for timeout; it returns with mu held, and whether the
// queue emptied. Caller MUST hold mu.
func (t *termImpl) idleLocked(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for len(t.queued) > 0 || t.writeSince.Load() != 0 {
		left := time.Until(deadline)
		if left <= 0 || t.finalized {
			return false
		}
		t.mu.Unlock()
		select {
		case <-t.drained:
		case <-time.After(left):
		}
		t.mu.Lock()
	}
	return true
}

// SetKeyReport hands the ask to the input reader, which answers it
func (t *termImpl) SetKeyReport(report KeyReport, allow KeySource) error {
	if report > KeyReportReleases {
		return fmt.Errorf("terminal: unknown key report %d", report)
	}
	t.mu.Lock()
	ready := t.initialized && !t.finalized
	t.mu.Unlock()
	if ready {
		t.input.askKeys(report, allow)
	}
	return nil
}

// EmergencyReset attempts to restore terminal to sane state
// Call this from panic recovery if Fini() cannot be called normally. Given a
// redirected stdout, it writes to the controlling terminal instead.
func EmergencyReset(w io.Writer) {
	// A console keyboard left raw types nothing a player can use
	restoreConsoleKeys()

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
	if keyProtoPushed.Load() {
		w.Write(csiKeyPop)
		w.Write(csiFocusOff)
	}

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
