package terminal

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// EventType distinguishes input event categories
type EventType uint8

const (
	EventKey EventType = iota
	EventResize
	EventPaste     // Bracketed paste, Text holds it
	EventMouse     // SGR mouse reporting
	EventError     // Read error
	EventClosed    // Input closed
	EventFocus     // The terminal gained or lost focus, while it reports releases through its protocol
	EventKeyReport // What a SetKeyReport ask came to: Report through Source
)

// Event represents a terminal input event
type Event struct {
	Type      EventType
	Key       Key
	Rune      rune
	Modifiers Modifier
	Action    KeyAction // For EventKey: KeyPress unless the terminal reports releases
	Lost      bool      // For a KeyRelease the package made for a hold whose own release can't arrive
	Width     int       // For EventResize
	Height    int       // For EventResize
	Err       error     // For EventError
	Text      string    // For EventPaste: the pasted bytes as sent, controls included
	Focused   bool      // For EventFocus: whether the terminal now has focus
	Report    KeyReport // For EventKeyReport: what the terminal now reports of each key
	Source    KeySource // For EventKeyReport: the source releases come from, zero for presses

	// Mouse event fields
	MouseX      int
	MouseY      int
	MouseBtn    MouseButton
	MouseAction MouseAction
	Touch       bool // A touch, from a host that reads one; terminals report none
}

// inputReader handles raw stdin parsing
type inputReader struct {
	backend Backend
	eventCh chan Event
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Persistent buffer for stream assembly, not fixed size zero-alloc to avoid corrupting partial UTF-8 at boundary
	buf []byte

	// Bracketed paste: whether SetPasteMode turned it on, and a paste in
	// progress, what has arrived of it and when its last bytes did
	pasteMode atomic.Bool
	pasting   bool
	paste     []byte
	pasteSeen time.Time

	// An over-long CSI sequence is being dropped up to its final byte
	dropCSI bool

	// Key reporting asks: the sources the backend has, the latest ask waiting
	// for the reader, and how the reader writes to the terminal, false once it
	// is closing; pushed is whether the protocol's flags are pushed after seq
	sources KeySource
	asks    chan keyAsk
	write   func(seq []byte, pushed bool) bool

	// The console the backend runs on, nil for none, and how the reader
	// switches its keyboard raw, and back
	console    keyConsole
	keysRaw    func() (ok, again bool)
	keysCooked func()

	// Key reporting, the reader's own: the ask it works toward and whether it
	// is still to be answered, what the terminal reports of each key and the
	// source releases come from, whether focus reports are parsed, and the
	// keys a source reports held down
	want   keyAsk
	answer bool
	report KeyReport
	source KeySource
	focus  bool
	holds  KeyHolds

	// The protocol: an exchange in flight and when it is given up, the flags
	// the terminal reported in it (-1 for none), whether the terminal has the
	// protocol (-1 no, 0 not known, 1 yes), and whether key reports are
	// decoded, from the push until the terminal has the pop
	exchange      protoExchange
	deadline      time.Time
	protoReported int
	protoHas      int8
	decoding      bool

	// The console: whether its keyboard refused to go raw, for good (-1) or
	// for this ask (-2), and the parser reading what it types for a key
	consoleHas int8
	typer      *inputReader

	mu      sync.Mutex
	running bool
}

// newInputReader creates a new input reader
func newInputReader(backend Backend) *inputReader {
	return &inputReader{
		backend: backend,
		eventCh: make(chan Event, 256),
		asks:    make(chan keyAsk, 1),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		buf:     make([]byte, 0, 256),
	}
}

// start begins reading input in a goroutine
func (r *inputReader) start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	go r.readLoop()
}

// stop signals the reader to stop
func (r *inputReader) stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	r.mu.Unlock()

	close(r.stopCh)
	// Wait with timeout - don't block forever if read is stuck
	select {
	case <-r.doneCh:
	case <-time.After(100 * time.Millisecond):
		// Reader stuck on blocking read, proceed anyway
	}
}

// events returns the event channel
func (r *inputReader) events() <-chan Event {
	return r.eventCh
}

// readLoop is the main input reading goroutine
func (r *inputReader) readLoop() {
	defer close(r.doneCh)

	// Panic recovery for raw input reader
	defer func() {
		if p := recover(); p != nil {
			EmergencyReset(writerAdapter{r.backend})
			// Use \r\n for clean output
			fmt.Fprintf(os.Stderr, "\r\n\x1b[31mINPUT READER CRASHED: %v\x1b[0m\r\n", p)
			fmt.Fprintf(os.Stderr, "Stack Trace:\r\n%s\r\n", debug.Stack())
			os.Exit(1)
		}
	}()

	for {
		r.serviceKeys()

		// Blocking read from backend
		data, err := r.backend.Read(r.stopCh)
		if err != nil {
			r.end()
			r.sendEvent(Event{Type: EventError, Err: err})
			return
		}

		if r.source == KeySourceConsole {
			r.consoleRead(data)
			select {
			case <-r.stopCh:
				r.end()
				r.sendEvent(Event{Type: EventClosed})
				return
			default:
				continue
			}
		}

		// A paste whose end marker never comes ends when input pauses, so
		// keys are never swallowed for good; a read may block (wasm)
		if r.pasting && time.Since(r.pasteSeen) > pasteIdle {
			r.paste = append(r.paste, r.buf...)
			r.buf = r.buf[:0]
			r.endPaste()
		}

		if len(data) == 0 {
			// Timeout (Unix poll) or empty read: a pending ESC, or two, was
			// pressed alone; inside a paste it is text
			if ev, ok := loneEscape[string(r.buf)]; ok && !r.pasting {
				r.sendKey(0, ev)
				r.buf = r.buf[:0]
			}
			select {
			case <-r.stopCh:
				r.end()
				r.sendEvent(Event{Type: EventClosed})
				return
			default:
				continue
			}
		}

		// Append to persistent buffer
		r.buf = append(r.buf, data...)

		// Parse as much as possible, get consumed count
		consumed := r.parseInput(r.buf)

		// Compact buffer
		if consumed > 0 {
			if consumed >= len(r.buf) {
				r.buf = r.buf[:0]
			} else {
				copy(r.buf, r.buf[consumed:])
				r.buf = r.buf[:len(r.buf)-consumed]
			}
		}
	}
}

// end releases what is held as input ends, and the console's keyboard
func (r *inputReader) end() {
	r.holds.ReleaseAll(r.sendEvent)
	if r.source == KeySourceConsole {
		r.console.cooked()
	}
}

// parseInput parses raw bytes into events and returns bytes consumed (stop on incomplete sequence)
func (r *inputReader) parseInput(data []byte) int {
	i := 0
	n := len(data)

	for i < n {
		select {
		case <-r.stopCh:
			return i
		default:
		}

		if r.dropCSI {
			for i < n && data[i] >= 0x20 && data[i] <= 0x3f {
				i++
			}
			if i < n {
				r.dropCSI = false
				if data[i] >= 0x40 && data[i] <= 0x7e {
					i++
				}
			}
			continue
		}

		if r.pasting {
			end := bytes.Index(data[i:], pasteEnd)
			if end < 0 {
				// Keep back what could start pasteEnd until more arrives
				keep := min(n-i, len(pasteEnd)-1)
				r.paste = append(r.paste, data[i:n-keep]...)
				r.pasteSeen = time.Now()
				return n - keep
			}
			r.paste = append(r.paste, data[i:i+end]...)
			r.endPaste()
			i += end + len(pasteEnd)
			continue
		}
		if r.pasteMode.Load() && bytes.HasPrefix(data[i:], pasteStart) {
			r.pasting, r.pasteSeen = true, time.Now()
			i += len(pasteStart)
			continue
		}

		b := data[i]

		// Fast path: printable ASCII
		if b >= 0x20 && b < 0x7f {
			r.sendKey(0, Event{Type: EventKey, Key: KeyRune, Rune: rune(b)})
			i++
			continue
		}

		// Escape sequence
		if b == 0x1b {
			// Need at least 2 bytes to determine sequence type
			if i+1 >= n {
				return i // Wait for more data
			}

			consumed, ev := r.parseEscape(data[i:])
			if consumed == 0 {
				// Incomplete sequence, wait for more data
				return i
			}

			// Only emit if not a swallowed unknown sequence
			if ev.Type != EventKey {
				r.sendEvent(ev)
			} else if ev.Key != KeyNone {
				r.sendKey(0, ev)
			}
			i += consumed
			continue
		}

		// Control characters
		if b < 0x20 {
			r.sendKey(0, r.parseControl(b))
			i++
			continue
		}

		// DEL
		if b == 0x7f {
			r.sendKey(0, Event{Type: EventKey, Key: KeyBackspace})
			i++
			continue
		}

		// UTF-8 multibyte
		if b >= 0x80 {
			// Check if full sequence available
			seqLen := utf8SeqLen(b)
			if seqLen == 0 {
				// Invalid start byte, skip
				i++
				continue
			}
			if i+seqLen > n {
				// Incomplete UTF-8, wait for more data
				return i
			}

			rn, size := decodeRune(data[i:])
			r.sendKey(0, Event{Type: EventKey, Key: KeyRune, Rune: rn})
			i += size
			continue
		}

		i++
	}
	return i
}

// loneEscape is what an ESC or two left when input paused was
var loneEscape = map[string]Event{
	"\x1b":     {Type: EventKey, Key: KeyEscape},
	"\x1b\x1b": {Type: EventKey, Key: KeyEscape, Modifiers: ModAlt},
}

// pasteIdle is how long a paste may pause before it is taken as ended
var pasteIdle = time.Second

// endPaste sends the paste gathered so far as one event
func (r *inputReader) endPaste() {
	r.sendEvent(Event{Type: EventPaste, Text: string(r.paste)})
	r.pasting, r.paste = false, r.paste[:0]
}

// utf8SeqLen returns expected UTF-8 sequence length from start byte, 0 if invalid
func utf8SeqLen(b byte) int {
	if b < 0x80 {
		return 1
	}
	if b&0xe0 == 0xc0 {
		return 2
	}
	if b&0xf0 == 0xe0 {
		return 3
	}
	if b&0xf8 == 0xf0 {
		return 4
	}
	return 0 // Invalid
}

// parseEscape attempts to parse an escape sequence, returns 0 on incomplete
func (r *inputReader) parseEscape(data []byte) (int, Event) {
	if len(data) < 2 {
		return 0, Event{} // Incomplete, wait for more
	}

	// ESC ESC: Escape before a paste, Alt and the sequence that follows (rxvt's
	// Alt+arrows), else Alt+Escape; alone it waits for the timeout
	if rest := data[1:]; rest[0] == 0x1b {
		switch {
		case len(rest) == 1:
			return 0, Event{}
		case bytes.HasPrefix(rest, pasteStart):
			return 1, Event{Type: EventKey, Key: KeyEscape}
		case rest[1] == 'O', rest[1] == '[' && csiLen(rest) != csiTyped:
			n, ev := r.parseEscape(rest)
			if n == 0 {
				return 0, Event{}
			}
			ev.Modifiers |= ModAlt
			return n + 1, ev
		}
		return 2, Event{Type: EventKey, Key: KeyEscape, Modifiers: ModAlt}
	}

	if data[1] == '[' {
		return r.parseCSI(data)
	}
	if data[1] == 'O' {
		return r.parseSS3(data)
	}

	// Alt+Control character (ESC + 0x00-0x1F)
	if data[1] < 0x20 {
		ev := r.parseControl(data[1])
		ev.Modifiers |= ModAlt
		return 2, ev
	}

	// Alt+printable
	if data[1] >= 0x20 && data[1] < 0x7f {
		return 2, Event{Type: EventKey, Key: KeyRune, Rune: rune(data[1]), Modifiers: ModAlt}
	}

	// Alt+Backspace
	if data[1] == 0x7f {
		return 2, Event{Type: EventKey, Key: KeyBackspace, Modifiers: ModAlt}
	}

	// Alt+multibyte character; a byte no character starts with leaves Escape
	size := utf8SeqLen(data[1])
	if size == 0 {
		return 1, Event{Type: EventKey, Key: KeyEscape}
	}
	if len(data) < 1+size {
		return 0, Event{}
	}
	rn, n := decodeRune(data[1:])
	return 1 + n, Event{Type: EventKey, Key: KeyRune, Rune: rn, Modifiers: ModAlt}
}

// csiMaxLen bounds a CSI sequence a terminal sends, with ESC [ and its final
// byte; a longer one is dropped up to its final byte
const csiMaxLen = 256

// csiLen results other than a length: more may be arriving, a byte no
// sequence holds came (ESC [ was typed, as Alt+[), or no final byte came
// within csiMaxLen
const (
	csiPartial  = 0
	csiTyped    = -1
	csiOverlong = -2
)

// csiLen returns the length of the CSI sequence data starts with, or why it
// has none yet. Parameter and intermediate bytes run to a final byte as in
// ECMA-48, but for the Linux console's F1-F5, ESC [ [ A to E.
func csiLen(data []byte) int {
	i := 2
	if len(data) > 2 && data[2] == '[' {
		i = 3
	}
	for ; i < len(data); i++ {
		if i == csiMaxLen {
			return csiOverlong
		}
		switch b := data[i]; {
		case b >= 0x40 && b <= 0x7e:
			return i + 1
		case b < 0x20 || b > 0x3f:
			return csiTyped
		}
	}
	return csiPartial
}

// parseCSI parses CSI sequence without allocation
func (r *inputReader) parseCSI(data []byte) (int, Event) {
	n := csiLen(data)
	switch {
	case n == csiPartial:
		return 0, Event{}
	case n == csiTyped:
		return 2, Event{Type: EventKey, Key: KeyRune, Rune: '[', Modifiers: ModAlt}
	case n == csiOverlong:
		r.dropCSI = true
		return csiMaxLen, Event{Type: EventKey, Key: KeyNone}
	case data[2] == '<' && (data[n-1] == 'M' || data[n-1] == 'm'):
		return n, parseSGRMouse(data[3:n-1], data[n-1])
	case r.focus && n == 3 && (data[2] == 'I' || data[2] == 'O'):
		// Holds end as focus leaves, since their releases go elsewhere
		if data[2] == 'O' {
			r.holds.ReleaseAll(r.sendEvent)
		}
		return n, Event{Type: EventFocus, Focused: data[2] == 'I'}
	case data[2] == '?' && data[n-1] == 'u':
		// The protocol's flags in force, answering a query
		if v, ok := parseKeyParams(data[3 : n-1]); ok {
			r.protoReply(int(max(v[0][0], 0)))
		}
		return n, Event{Type: EventKey, Key: KeyNone}
	case data[2] == '?' && data[n-1] == 'c':
		// Device attributes, which close an exchange: the terminal has
		// answered all asked before them
		r.exchangeDone(true)
		return n, Event{Type: EventKey, Key: KeyNone}
	case r.decoding && r.protoKey(data[2:n-1], data[n-1]):
		return n, Event{Type: EventKey, Key: KeyNone}
	}

	if key, mod, ok := lookupCSI(data[2:n]); ok {
		return n, Event{Type: EventKey, Key: key, Modifiers: mod}
	}

	// Unknown but valid CSI syntax - consume and return KeyNone
	return n, Event{Type: EventKey, Key: KeyNone}
}

// parseSS3 parses SS3 sequence without allocation, returns length even for unknown sequences
func (r *inputReader) parseSS3(data []byte) (int, Event) {
	if len(data) < 3 {
		return 0, Event{}
	}
	if key, mod, ok := lookupSS3(data[2:3]); ok {
		ev := Event{Type: EventKey, Key: key, Modifiers: mod}
		if r.decoding && key != KeyRune {
			// A terminal reporting by the protocol names this key too
			r.sendKey(protoNamed+uint32(key), ev)
			return 3, Event{Type: EventKey, Key: KeyNone}
		}
		return 3, ev
	}
	// Unknown SS3 - consume to prevent garbage
	return 3, Event{Type: EventKey, Key: KeyNone}
}

// parseControl maps control characters to keys
func (r *inputReader) parseControl(b byte) Event {
	switch b {
	case 0x00: // Ctrl+Space or Ctrl+@
		return Event{Type: EventKey, Key: KeyCtrlSpace}
	case 0x01:
		return Event{Type: EventKey, Key: KeyCtrlA}
	case 0x02:
		return Event{Type: EventKey, Key: KeyCtrlB}
	case 0x03:
		return Event{Type: EventKey, Key: KeyCtrlC}
	case 0x04:
		return Event{Type: EventKey, Key: KeyCtrlD}
	case 0x05:
		return Event{Type: EventKey, Key: KeyCtrlE}
	case 0x06:
		return Event{Type: EventKey, Key: KeyCtrlF}
	case 0x07:
		return Event{Type: EventKey, Key: KeyCtrlG}
	case 0x08: // Ctrl+H or Backspace
		return Event{Type: EventKey, Key: KeyBackspace}
	case 0x09: // Tab
		return Event{Type: EventKey, Key: KeyTab}
	case 0x0a, 0x0d: // LF, CR (Enter)
		return Event{Type: EventKey, Key: KeyEnter}
	case 0x0b:
		return Event{Type: EventKey, Key: KeyCtrlK}
	case 0x0c:
		return Event{Type: EventKey, Key: KeyCtrlL}
	case 0x0e:
		return Event{Type: EventKey, Key: KeyCtrlN}
	case 0x0f:
		return Event{Type: EventKey, Key: KeyCtrlO}
	case 0x10:
		return Event{Type: EventKey, Key: KeyCtrlP}
	case 0x11:
		return Event{Type: EventKey, Key: KeyCtrlQ}
	case 0x12:
		return Event{Type: EventKey, Key: KeyCtrlR}
	case 0x13:
		return Event{Type: EventKey, Key: KeyCtrlS}
	case 0x14:
		return Event{Type: EventKey, Key: KeyCtrlT}
	case 0x15:
		return Event{Type: EventKey, Key: KeyCtrlU}
	case 0x16:
		return Event{Type: EventKey, Key: KeyCtrlV}
	case 0x17:
		return Event{Type: EventKey, Key: KeyCtrlW}
	case 0x18:
		return Event{Type: EventKey, Key: KeyCtrlX}
	case 0x19:
		return Event{Type: EventKey, Key: KeyCtrlY}
	case 0x1a:
		return Event{Type: EventKey, Key: KeyCtrlZ}
	case 0x1b: // ESC (shouldn't reach here normally)
		return Event{Type: EventKey, Key: KeyEscape}
	case 0x1c:
		return Event{Type: EventKey, Key: KeyCtrlBackslash}
	case 0x1d:
		return Event{Type: EventKey, Key: KeyCtrlBracketRight}
	case 0x1e:
		return Event{Type: EventKey, Key: KeyCtrlCaret}
	case 0x1f:
		return Event{Type: EventKey, Key: KeyCtrlUnderscore}
	}
	return Event{Type: EventKey, Key: KeyNone}
}

// parseSGRMouse parses mouse SGR sequences, ESC [ < Btn ; X ; Y M/m, from
// the parameters and final byte; a malformed one, or a button past 9, is
// dropped as KeyNone
func parseSGRMouse(params []byte, final byte) Event {
	btn, x, y, ok := parseSGRParams(params)
	if !ok {
		return Event{Type: EventKey, Key: KeyNone}
	}

	ev := Event{Type: EventMouse, MouseX: x - 1, MouseY: y - 1} // Convert to 0-indexed

	// Bits 0-1 number a button within its bank: the main three and none, the
	// wheel's four directions (bit 6), or buttons 8 to 11 (bit 7); bit 5 is motion
	id := btn & 0x03
	switch btn & (64 | 128) {
	case 0:
		ev.MouseBtn = [...]MouseButton{MouseBtnLeft, MouseBtnMiddle, MouseBtnRight, MouseBtnNone}[id]
	case 64:
		ev.MouseBtn = [...]MouseButton{MouseBtnWheelUp, MouseBtnWheelDown, MouseBtnWheelLeft, MouseBtnWheelRight}[id]
	case 128:
		if id > 1 {
			return Event{Type: EventKey, Key: KeyNone}
		}
		ev.MouseBtn = MouseBtnBack + MouseButton(id)
	default:
		return Event{Type: EventKey, Key: KeyNone}
	}

	switch {
	case btn&64 != 0:
		ev.MouseAction = MouseActionPress // A wheel step is its press
	case final == 'm':
		ev.MouseAction = MouseActionRelease
	case btn&32 == 0:
		ev.MouseAction = MouseActionPress
	case ev.MouseBtn != MouseBtnNone:
		ev.MouseAction = MouseActionDrag
	default:
		ev.MouseAction = MouseActionMove
	}

	// Extract modifiers from button byte
	if btn&4 != 0 {
		ev.Modifiers |= ModShift
	}
	if btn&8 != 0 {
		ev.Modifiers |= ModAlt
	}
	if btn&16 != 0 {
		ev.Modifiers |= ModCtrl
	}

	return ev
}

// parseSGRParams extracts btn, x, y from "Btn;X;Y" format
func parseSGRParams(data []byte) (btn, x, y int, ok bool) {
	state := 0 // 0=btn, 1=x, 2=y
	val := 0

	for _, b := range data {
		if b == ';' {
			switch state {
			case 0:
				btn = val
			case 1:
				x = val
			}
			state++
			val = 0
			if state > 2 {
				return 0, 0, 0, false
			}
		} else if b >= '0' && b <= '9' {
			val = val*10 + int(b-'0')
			if val > 9999 { // Sanity limit
				return 0, 0, 0, false
			}
		} else {
			return 0, 0, 0, false
		}
	}

	if state != 2 {
		return 0, 0, 0, false
	}
	y = val
	return btn, x, y, true
}

// sendKey sends a key event at the level the terminal reports. code names the
// physical key the event came from, zero for one the terminal names none for:
// at releases such a key can't be held, so its release follows at once.
func (r *inputReader) sendKey(code uint32, ev Event) {
	switch {
	case r.report == KeyReportPresses:
		if ev.Action != KeyRelease {
			ev.Action = KeyPress
			r.sendEvent(ev)
		}
	case code == 0:
		ev.Action = KeyPress
		r.sendEvent(ev)
		ev.Action = KeyRelease
		r.sendEvent(ev)
	default:
		r.holds.Report(code, ev, r.sendEvent)
	}
}

// sendEvent sends an event to the channel, non-blocking
func (r *inputReader) sendEvent(ev Event) {
	select {
	case r.eventCh <- ev:
	default:
		// Channel full, drop event (shouldn't happen with 64 buffer)
	}
}

// decodeRune decodes the first UTF-8 rune from data
func decodeRune(data []byte) (rune, int) {
	if len(data) == 0 {
		return 0, 0
	}

	b := data[0]
	if b < 0x80 {
		return rune(b), 1
	}

	var size int
	var min rune
	var r rune

	switch {
	case b&0xe0 == 0xc0:
		size = 2
		min = 0x80
		r = rune(b & 0x1f)
	case b&0xf0 == 0xe0:
		size = 3
		min = 0x800
		r = rune(b & 0x0f)
	case b&0xf8 == 0xf0:
		size = 4
		min = 0x10000
		r = rune(b & 0x07)
	default:
		return 0xFFFD, 1 // Invalid, return replacement char
	}

	if len(data) < size {
		return 0xFFFD, 1
	}

	for i := 1; i < size; i++ {
		if data[i]&0xc0 != 0x80 {
			return 0xFFFD, 1
		}
		r = r<<6 | rune(data[i]&0x3f)
	}

	if r < min {
		return 0xFFFD, 1 // Overlong encoding
	}

	return r, size
}

// ControlKey is the key a terminal reads for Ctrl with a printable character: the
// control byte that pair sends, parsed as input parsing reads it. Letters of
// either case, space and '@', '[' '\' ']' '^' '_', '-' and '/' as '_', and the
// digits 2 to 7 have one; ok is false for any other character.
func ControlKey(r rune) (Key, bool) {
	var b byte
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		b = byte(r) & 0x1f
	case r == ' ', r == '@', r == '2':
		b = 0x00
	case r == '[', r == '3':
		b = 0x1b
	case r == '\\', r == '4':
		b = 0x1c
	case r == ']', r == '5':
		b = 0x1d
	case r == '^', r == '6':
		b = 0x1e
	case r == '_', r == '-', r == '/', r == '7':
		b = 0x1f
	default:
		return KeyNone, false
	}
	return (*inputReader)(nil).parseControl(b).Key, true
}
