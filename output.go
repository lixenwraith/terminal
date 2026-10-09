package terminal

import (
	"bufio"
	"unicode"
	"unicode/utf8"

	"github.com/lixenwraith/color"
)

// outputBuffer manages double-buffered terminal output with diffing
type outputBuffer struct {
	front     []Cell
	width     int
	height    int
	colorMode ColorMode
	writer    *bufio.Writer

	cursorX     int
	cursorY     int
	cursorValid bool

	// Style state for coalescing
	lastFg    color.RGB
	lastBg    color.RGB
	lastAttr  Attr
	lastValid bool
}

// writerAdapter adapts Backend to io.Writer for bufio
type writerAdapter struct {
	b Backend
}

func (wa writerAdapter) Write(p []byte) (int, error) {
	err := wa.b.Write(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// newOutputBuffer creates a new output buffer
func newOutputBuffer(backend Backend, colorMode ColorMode) *outputBuffer {
	// Use 128KB buffer for minimal calls to backend
	adapter := writerAdapter{b: backend}
	return &outputBuffer{
		writer:    bufio.NewWriterSize(adapter, 131072),
		colorMode: colorMode,
	}
}

// resize updates buffer dimensions
func (o *outputBuffer) resize(width, height int) {
	size := width * height
	if cap(o.front) < size {
		o.front = make([]Cell, size)
	} else {
		o.front = o.front[:size]
	}
	o.width = width
	o.height = height

	for i := range o.front {
		o.front[i] = Cell{Rune: 0}
	}
	o.lastValid = false
	o.cursorValid = false
}

// cellEqual compares two cells for equality (standalone for inlining)
func cellEqual(a, b Cell) bool {
	// A cell is only equal if every visual component matches, checking most likely changed fields first (Rune/Bg)
	return a.Rune == b.Rune &&
		a.Bg == b.Bg &&
		a.Fg == b.Fg &&
		a.Attrs == b.Attrs
}

// flush writes the back buffer to terminal, diffing against front buffer
func (o *outputBuffer) flush(cells []Cell, width, height int) {
	if width != o.width || height != o.height {
		o.resize(width, height)
	}

	expectedSize := width * height
	if len(cells) < expectedSize {
		return
	}

	w := o.writer

	for y := 0; y < height; y++ {
		rowStart := y * width

		// Early termination: find last dirty cell in row (scan backward)
		rowEnd := width
		for rowEnd > 0 && cellEqual(cells[rowStart+rowEnd-1], o.front[rowStart+rowEnd-1]) {
			rowEnd--
		}
		if rowEnd == 0 {
			continue // Entire row unchanged
		}

		x := 0
		for x < rowEnd {
			idx := rowStart + x

			if cellEqual(cells[idx], o.front[idx]) {
				x++
				continue
			}

			// Found dirty cell - check for small gaps ahead to potentially merge segments
			segStart := x
			segEnd := x + 1

			// Extend segment through small gaps (≤3 unchanged cells)
			for segEnd < rowEnd {
				// Find gap size
				gapStart := segEnd
				for gapStart < rowEnd && cellEqual(cells[rowStart+gapStart], o.front[rowStart+gapStart]) {
					gapStart++
				}
				gapSize := gapStart - segEnd

				if gapSize == 0 {
					// No gap, extend to next unchanged
					for segEnd < rowEnd && !cellEqual(cells[rowStart+segEnd], o.front[rowStart+segEnd]) {
						segEnd++
					}
					continue
				}

				if gapSize > 3 {
					break // Gap too large, end segment here
				}

				// Gap logic check: only bridge the gap if the gap cells have the same attributes as the current segment, otherwise, emit SGR codes inside the gap, making it more expensive than a cursor move
				gapCompatible := true
				refCell := cells[rowStart+segEnd-1] // The last dirty cell of the current segment

				for k := 0; k < gapSize; k++ {
					gCell := cells[rowStart+segEnd+k]
					// Strict equality on style/color to ensure no SGR emission
					if gCell.Fg != refCell.Fg || gCell.Bg != refCell.Bg || gCell.Attrs != refCell.Attrs {
						gapCompatible = false
						break
					}
				}

				if !gapCompatible {
					break // Gap has different style, cheaper to jump
				}

				// Check if there's more dirty content after gap
				if gapStart >= rowEnd {
					break // Gap extends to row end
				}

				// Small gap with content after - include gap in segment
				segEnd = gapStart
				// Continue to find more dirty cells
				for segEnd < rowEnd && !cellEqual(cells[rowStart+segEnd], o.front[rowStart+segEnd]) {
					segEnd++
				}
			}

			// Positions cursor to segment start
			o.moveCursorTo(w, segStart, y)

			// Write segment [segStart, segEnd)
			for sx := segStart; sx < segEnd; sx++ {
				cidx := rowStart + sx
				c := cells[cidx]

				o.writeStyleCoalesced(w, c.Fg, c.Bg, c.Attrs)

				// A cell holds one glyph, never a control the terminal would act on
				r := c.Rune
				if unicode.IsControl(r) || !utf8.ValidRune(r) {
					r = ' '
				}
				if r < 0x80 {
					w.WriteByte(byte(r))
				} else {
					w.WriteRune(r)
				}

				o.front[cidx] = c
				o.cursorX++
			}

			x = segEnd
		}
	}

	w.Write(csiSGR0)
	o.lastValid = false
	w.Flush()
}

// cursorForwardCost returns byte cost of cursor forward sequence
func cursorForwardCost(n int) int {
	if n == 1 {
		return 3 // \x1b[C
	}
	return 3 + digitCount(n) // \x1b[nC
}

// cursorAbsoluteCost returns byte cost of absolute cursor position
func cursorAbsoluteCost(x, y int) int {
	// \x1b[row;colH = 2 + digits(row) + 1 + digits(col) + 1
	return 4 + digitCount(y+1) + digitCount(x+1)
}

// digitCount returns number of decimal digits in n
func digitCount(n int) int {
	if n < 10 {
		return 1
	}
	if n < 100 {
		return 2
	}
	if n < 1000 {
		return 3
	}
	return 4
}

// moveCursorTo positions cursor using most efficient method
func (o *outputBuffer) moveCursorTo(w *bufio.Writer, x, y int) {
	if o.cursorValid && o.cursorX == x && o.cursorY == y {
		return
	}

	moved := false
	if o.cursorValid && o.cursorY == y && x > o.cursorX {
		gap := x - o.cursorX
		fwdCost := cursorForwardCost(gap)
		absCost := cursorAbsoluteCost(x, y)

		if fwdCost < absCost {
			writeCursorForward(w, gap)
			moved = true
		}
	}

	if !moved {
		writeCursorPos(w, x, y)
	}

	o.cursorX = x
	o.cursorY = y
	o.cursorValid = true
}

// writeStyleCoalesced emits a single combined SGR sequence when style changes
func (o *outputBuffer) writeStyleCoalesced(w *bufio.Writer, fg, bg color.RGB, attr Attr) {
	if o.colorMode == ColorModeNone {
		fg, bg, attr = color.RGB{}, color.RGB{}, attr&AttrStyle
	}
	// Check what changed
	fgChanged := !o.lastValid || fg != o.lastFg || (attr&AttrFgColor) != (o.lastAttr&AttrFgColor)
	bgChanged := !o.lastValid || bg != o.lastBg || (attr&AttrBgColor) != (o.lastAttr&AttrBgColor)
	styleAttr := attr & AttrStyle
	attrChanged := !o.lastValid || styleAttr != o.lastAttr&AttrStyle
	// A text console's bright colors (90-97) set bold, which only a reset
	// clears: 16 colors restate the whole style from one
	if o.colorMode == ColorMode16 && (fgChanged || bgChanged) {
		attrChanged = true
	}

	if !fgChanged && !bgChanged && !attrChanged {
		return
	}

	w.Write(csi)
	// If attributes changed, must reset first, then restate both colors
	if attrChanged {
		w.WriteByte('0')
		for _, a := range attrCodes {
			if styleAttr&a.bit != 0 {
				w.WriteByte(';')
				w.WriteByte(a.code)
			}
		}
		fgChanged, bgChanged = true, true
	}
	if fgChanged {
		o.writeColor(w, fg, attr&AttrFg256 != 0, attr&AttrFgDefault != 0, 30, attrChanged)
	}
	if bgChanged {
		o.writeColor(w, bg, attr&AttrBg256 != 0, attr&AttrBgDefault != 0, 40, attrChanged || fgChanged)
	}
	w.WriteByte('m')

	o.lastFg = fg
	o.lastBg = bg
	o.lastAttr = attr
	o.lastValid = true
}

// attrCodes pairs each style bit with its SGR parameter, in emission order
var attrCodes = [...]struct {
	bit  Attr
	code byte
}{{AttrBold, '1'}, {AttrDim, '2'}, {AttrItalic, '3'}, {AttrUnderline, '4'}, {AttrBlink, '5'}, {AttrReverse, '7'}}

// writeColor writes one color's SGR parameters, after a ';' when sep, for
// base 30 (foreground) or 40 (background): the terminal's own, a palette
// index, or RGB, as the color mode allows. ColorModeNone writes nothing.
func (o *outputBuffer) writeColor(w *bufio.Writer, c color.RGB, palette, deflt bool, base int, sep bool) {
	if o.colorMode == ColorModeNone {
		return
	}
	if sep {
		w.WriteByte(';')
	}
	switch {
	case deflt:
		writeInt(w, base+9)
	case o.colorMode == ColorMode16:
		i := color.RGBTo16(c)
		if palette {
			i = paletteTo16(c.R)
		}
		if i >= 8 {
			base += 60 - 8 // bright: 90-97, 100-107
		}
		writeInt(w, base+int(i))
	case palette:
		writeInt(w, base+8)
		w.Write(sgrPalette)
		writeInt(w, int(c.R))
	case o.colorMode == ColorModeTrueColor:
		writeInt(w, base+8)
		w.Write(sgrRGB)
		writeInt(w, int(c.R))
		w.WriteByte(';')
		writeInt(w, int(c.G))
		w.WriteByte(';')
		writeInt(w, int(c.B))
	default:
		writeInt(w, base+8)
		w.Write(sgrPalette)
		writeInt(w, int(RGBTo256(c)))
	}
}

// forceFullRedraw clears front buffer to force complete redraw
func (o *outputBuffer) forceFullRedraw() {
	for i := range o.front {
		o.front[i] = Cell{Rune: 0}
	}
	o.lastValid = false
	o.cursorValid = false
}

// clear writes a clear screen with specified background, attr's
// AttrBgDefault or AttrBg256 saying how to read it
func (o *outputBuffer) clear(bg color.RGB, attr Attr) {
	w := o.writer
	w.Write(csiSGR0)
	w.Write(csi)
	o.writeColor(w, bg, attr&AttrBg256 != 0, attr&AttrBgDefault != 0, 40, false)
	w.WriteByte('m')
	w.Write(csiClear)

	o.lastValid = false
	o.cursorValid = false
	w.Flush()

	for i := range o.front {
		o.front[i] = Cell{Rune: ' ', Bg: bg, Attrs: attr & AttrBgColor}
	}
}

// invalidateCursor marks cursor position as unknown
func (o *outputBuffer) invalidateCursor() {
	o.cursorValid = false
}
