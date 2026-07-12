package terminal

import (
	"bufio"
)

// Pre-allocated ANSI sequence fragments (avoid allocations during render)
var (
	// CSI sequences
	csi      = []byte("\x1b[")
	csiEnd   = []byte("m")
	csiReset = []byte("\x1b[0m")
	csiClear = []byte("\x1b[2J\x1b[H")
	csiHome  = []byte("\x1b[H")
	csiRIS   = []byte("\x1bc") // Reset to Initial GameState (emergency)
	csiSGR0  = []byte("\x1b[0m")

	// Cursor control
	csiCursorHide = []byte("\x1b[?25l")
	csiCursorShow = []byte("\x1b[?25h")
	csiCursorPos  = []byte("\x1b[") // followed by row;colH

	// Screen modes
	csiAltScreenEnter = []byte("\x1b[?1049h")
	csiAltScreenExit  = []byte("\x1b[?1049l")
	// DECAWM: Auto-Wrap Mode
	// ?7l disables wrapping (cursor sticks at right edge), preventing scroll when writing to bottom-right corner
	csiAutoWrapOn  = []byte("\x1b[?7h")
	csiAutoWrapOff = []byte("\x1b[?7l")

	// Color prefixes
	csiFg256     = []byte("\x1b[38;5;") // followed by N;m
	csiBg256     = []byte("\x1b[48;5;") // followed by N;m
	csiFgRGB     = []byte("\x1b[38;2;") // followed by R;G;B;m
	csiBgRGB     = []byte("\x1b[48;2;") // followed by R;G;B;m
	csiDefaultFg = []byte("\x1b[39m")
	csiDefaultBg = []byte("\x1b[49m")

	// Attribute sequences
	csiAttrBold      = []byte("\x1b[1m")
	csiAttrDim       = []byte("\x1b[2m")
	csiAttrItalic    = []byte("\x1b[3m")
	csiAttrUnderline = []byte("\x1b[4m")
	csiAttrBlink     = []byte("\x1b[5m")
	csiAttrReverse   = []byte("\x1b[7m")

	// Mouse mode sequences (SGR 1006 for extended coordinates)
	csiMouseClickOn   = []byte("\x1b[?1000h") // Enable click reporting
	csiMouseClickOff  = []byte("\x1b[?1000l")
	csiMouseDragOn    = []byte("\x1b[?1002h") // Enable button-event (drag) tracking
	csiMouseDragOff   = []byte("\x1b[?1002l")
	csiMouseMotionOn  = []byte("\x1b[?1003h") // Enable any-event (all motion) tracking
	csiMouseMotionOff = []byte("\x1b[?1003l")
	csiMouseSGROn     = []byte("\x1b[?1006h") // Enable SGR extended mode
	csiMouseSGROff    = []byte("\x1b[?1006l")
)

// writeInt writes an integer without allocation
// Optimized for terminal values (0-255 common, 0-999 typical max)
func writeInt(w *bufio.Writer, n int) {
	if n < 0 {
		n = 0
	}
	if n < 10 {
		w.WriteByte(byte(n) + '0')
		return
	}
	if n < 100 {
		w.WriteByte(byte(n/10) + '0')
		w.WriteByte(byte(n%10) + '0')
		return
	}
	if n < 1000 {
		w.WriteByte(byte(n/100) + '0')
		w.WriteByte(byte(n/10%10) + '0')
		w.WriteByte(byte(n%10) + '0')
		return
	}
	// Fallback for >999 (rare)
	var buf [5]byte
	i := 4
	for n > 0 {
		buf[i] = byte(n%10) + '0'
		n /= 10
		i--
	}
	w.Write(buf[i+1:])
}

// writeCursorPos writes cursor positioning sequence (0-indexed input)
func writeCursorPos(w *bufio.Writer, x, y int) {
	w.Write(csiCursorPos)
	writeInt(w, y+1)
	w.WriteByte(';')
	writeInt(w, x+1)
	w.WriteByte('H')
}

// writeCursorForward writes cursor forward N positions
func writeCursorForward(w *bufio.Writer, n int) {
	if n <= 0 {
		return
	}
	if n == 1 {
		w.Write([]byte("\x1b[C"))
		return
	}
	w.Write(csi)
	writeInt(w, n)
	w.WriteByte('C')
}