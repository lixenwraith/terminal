package terminal

import "strconv"

// Pre-allocated ANSI sequence fragments (avoid allocations during render)
var (
	// CSI sequences
	csi      = []byte("\x1b[")
	csiClear = []byte("\x1b[2J\x1b[H")
	csiRIS   = []byte("\x1bc") // Reset to Initial State (emergency)
	csiSGR0  = []byte("\x1b[0m")

	// Cursor control
	csiCursorHide = []byte("\x1b[?25l")
	csiCursorShow = []byte("\x1b[?25h")
	csiCursorPos  = []byte("\x1b[") // followed by row;colH

	// Screen modes; the alternate screen also turns off the terminal's own bidi
	// reordering (ECMA-48 BDSM, mode 8), so right-to-left text keeps its cells
	csiAltScreenEnter = []byte("\x1b[?1049h\x1b[8l")
	csiAltScreenExit  = []byte("\x1b[?1049l\x1b[8h")
	// Synchronized output (DEC mode 2026): a terminal that has it shows what lies
	// between the two at once, so it never draws half a frame; others ignore both
	csiSyncBegin = []byte("\x1b[?2026h")
	csiSyncEnd   = []byte("\x1b[?2026l")

	// DECAWM: Auto-Wrap Mode
	// ?7l disables wrapping (cursor sticks at right edge), preventing scroll when writing to bottom-right corner
	csiAutoWrapOn  = []byte("\x1b[?7h")
	csiAutoWrapOff = []byte("\x1b[?7l")

	// Mouse mode sequences (SGR 1006 for extended coordinates)
	csiMouseClickOn   = []byte("\x1b[?1000h") // Enable click reporting
	csiMouseClickOff  = []byte("\x1b[?1000l")
	csiMouseDragOn    = []byte("\x1b[?1002h") // Enable button-event (drag) tracking
	csiMouseDragOff   = []byte("\x1b[?1002l")
	csiMouseMotionOn  = []byte("\x1b[?1003h") // Enable any-event (all motion) tracking
	csiMouseMotionOff = []byte("\x1b[?1003l")
	csiMouseSGROn     = []byte("\x1b[?1006h") // Enable SGR extended mode
	csiMouseSGROff    = []byte("\x1b[?1006l")

	// Bracketed paste: the terminal wraps pasted text in pasteStart, pasteEnd
	csiPasteOn  = []byte("\x1b[?2004h")
	csiPasteOff = []byte("\x1b[?2004l")
	pasteStart  = []byte("\x1b[200~")
	pasteEnd    = []byte("\x1b[201~")
)

// decimal holds each byte value's digits, then how many there are in its last
// byte, so a value appends as one store of three bytes, with no branch
var decimal = func() (t [256][4]byte) {
	for v := range t {
		n := copy(t[v][:3], strconv.Itoa(v))
		t[v][3] = byte(n)
	}
	return t
}()

// appendU8 appends v in decimal
func appendU8(b []byte, v uint8) []byte {
	d := &decimal[v]
	n := len(b) + int(d[3])
	return append(b, d[0], d[1], d[2])[:n]
}

// appendInt appends n in decimal, a negative n as 0
func appendInt(b []byte, n int) []byte {
	if uint(n) < 256 {
		return appendU8(b, uint8(n))
	}
	return strconv.AppendInt(b, int64(max(n, 0)), 10)
}

// appendCursorPos appends cursor positioning sequence (0-indexed input)
func appendCursorPos(b []byte, x, y int) []byte {
	b = appendInt(append(b, csiCursorPos...), y+1)
	return append(appendInt(append(b, ';'), x+1), 'H')
}

// appendCursorForward appends cursor forward N positions
func appendCursorForward(b []byte, n int) []byte {
	if n <= 0 {
		return b
	}
	if n == 1 {
		return append(b, "\x1b[C"...)
	}
	return append(appendInt(append(b, csi...), n), 'C')
}
