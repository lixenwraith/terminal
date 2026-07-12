package terminal

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// EventType distinguishes input event categories
type EventType uint8

const (
	EventKey EventType = iota
	EventResize
	EventPaste  // Future: bracketed paste
	EventMouse  // SGR mouse reporting
	EventError  // Read error
	EventClosed // Input closed
)

// Event represents a terminal input event
type Event struct {
	Type      EventType
	Key       Key
	Rune      rune
	Modifiers Modifier
	Width     int   // For EventResize
	Height    int   // For EventResize
	Err       error // For EventError

	// Mouse event fields
	MouseX      int
	MouseY      int
	MouseBtn    MouseButton
	MouseAction MouseAction
}

// inputReader handles raw stdin parsing
type inputReader struct {
	backend Backend
	eventCh chan Event
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Persistent buffer for stream assembly, not fixed size zero-alloc to avoid corrupting partial UTF-8 at boundary
	buf []byte

	mu      sync.Mutex
	running bool
}

// newInputReader creates a new input reader
func newInputReader(backend Backend) *inputReader {
	return &inputReader{
		backend: backend,
		eventCh: make(chan Event, 256),
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
		if r := recover(); r != nil {
			EmergencyReset(os.Stdout)
			// Use \r\n for clean output
			fmt.Fprintf(os.Stderr, "\r\n\x1b[31mINPUT READER CRASHED: %v\x1b[0m\r\n", r)
			fmt.Fprintf(os.Stderr, "Stack Trace:\r\n%s\r\n", debug.Stack())
			os.Exit(1)
		}
	}()

	for {
		// Blocking read from backend
		data, err := r.backend.Read(r.stopCh)
		if err != nil {
			r.sendEvent(Event{Type: EventError, Err: err})
			return
		}

		if len(data) == 0 {
			// Timeout (Unix poll) or empty read
			// Emit pending standalone ESC if present
			if len(r.buf) == 1 && r.buf[0] == 0x1b {
				r.sendEvent(Event{Type: EventKey, Key: KeyEscape})
				r.buf = r.buf[:0]
			}
			select {
			case <-r.stopCh:
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

		b := data[i]

		// Fast path: printable ASCII
		if b >= 0x20 && b < 0x7f {
			r.sendEvent(Event{Type: EventKey, Key: KeyRune, Rune: rune(b)})
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
			if ev.Key != KeyNone || ev.Type != EventKey {
				r.sendEvent(ev)
			}
			i += consumed
			continue
		}

		// Control characters
		if b < 0x20 {
			r.sendEvent(r.parseControl(b))
			i++
			continue
		}

		// DEL
		if b == 0x7f {
			r.sendEvent(Event{Type: EventKey, Key: KeyBackspace})
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
			r.sendEvent(Event{Type: EventKey, Key: KeyRune, Rune: rn})
			i += size
			continue
		}

		i++
	}
	return i
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

	// ESC ESC -> Alt+Escape
	if data[1] == 0x1b {
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

	return 0, Event{}
}

// parseCSI parses CSI sequence without allocation
func (r *inputReader) parseCSI(data []byte) (int, Event) {
	if len(data) < 3 {
		return 0, Event{}
	}

	// SGR mouse: ESC [ < Btn ; X ; Y M/m
	if data[2] == '<' {
		return r.parseSGRMouse(data)
	}

	end := 2
	maxScan := len(data)
	if maxScan > 16 {
		maxScan = 16
	}

	for end < maxScan {
		b := data[end]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
			end++
			break
		}
		if b < 0x20 || b > 0x7e {
			return 0, Event{}
		}
		end++
	}

	// Check if we found a terminator or ran out of data
	if end <= 2 || end > maxScan {
		return 0, Event{} // Incomplete
	}

	// Check last byte is valid terminator
	lastByte := data[end-1]
	if !((lastByte >= 'A' && lastByte <= 'Z') || (lastByte >= 'a' && lastByte <= 'z') || lastByte == '~') {
		return 0, Event{} // Incomplete, no terminator found
	}

	if key, mod, ok := lookupCSI(data[2:end]); ok {
		return end, Event{Type: EventKey, Key: key, Modifiers: mod}
	}

	// Unknown but valid CSI syntax - consume and return KeyNone
	return end, Event{Type: EventKey, Key: KeyNone}
}

// parseSS3 parses SS3 sequence without allocation, returns length even for unknown sequences
func (r *inputReader) parseSS3(data []byte) (int, Event) {
	if len(data) < 3 {
		return 0, Event{}
	}
	if key, mod, ok := lookupSS3(data[2:3]); ok {
		return 3, Event{Type: EventKey, Key: key, Modifiers: mod}
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

// parseSGRMouse parses mouse SGR sequences
func (r *inputReader) parseSGRMouse(data []byte) (int, Event) {
	// Format: ESC [ < Btn ; X ; Y M/m
	// Minimum: ESC [ < 0 ; 1 ; 1 M = 10 bytes
	if len(data) < 10 {
		return 0, Event{}
	}

	// Find terminator M or m
	end := 3
	for end < len(data) && end < 32 {
		if data[end] == 'M' || data[end] == 'm' {
			break
		}
		end++
	}
	if end >= len(data) || (data[end] != 'M' && data[end] != 'm') {
		return 0, Event{}
	}

	// Parse: Btn;X;Y
	params := data[3:end]
	btn, x, y, ok := parseSGRParams(params)
	if !ok {
		return 0, Event{}
	}

	ev := Event{Type: EventMouse, MouseX: x - 1, MouseY: y - 1} // Convert to 0-indexed

	// Decode button and action
	// Bits 0-1: button (0=left, 1=middle, 2=right, 3=release)
	// Bit 5 (32): motion
	// Bit 6 (64): scroll
	buttonID := btn & 0x03
	isMotion := btn&32 != 0
	isScroll := btn&64 != 0

	if isScroll {
		// Scroll: buttonID 0=up, 1=down
		if buttonID == 0 {
			ev.MouseBtn = MouseBtnWheelUp
		} else {
			ev.MouseBtn = MouseBtnWheelDown
		}
		ev.MouseAction = MouseActionPress // Scroll is instantaneous
	} else {
		// Regular button
		switch buttonID {
		case 0:
			ev.MouseBtn = MouseBtnLeft
		case 1:
			ev.MouseBtn = MouseBtnMiddle
		case 2:
			ev.MouseBtn = MouseBtnRight
		case 3:
			ev.MouseBtn = MouseBtnNone // Release with no specific button
		}

		if data[end] == 'M' {
			if isMotion {
				if ev.MouseBtn != MouseBtnNone {
					ev.MouseAction = MouseActionDrag
				} else {
					ev.MouseAction = MouseActionMove
				}
			} else {
				ev.MouseAction = MouseActionPress
			}
		} else {
			ev.MouseAction = MouseActionRelease
		}
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

	return end + 1, ev
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
