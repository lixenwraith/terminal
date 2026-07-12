package terminal

import (
	"bufio"
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
	lastFg    RGB
	lastBg    RGB
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

				r := c.Rune
				if r == 0 {
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
func (o *outputBuffer) writeStyleCoalesced(w *bufio.Writer, fg, bg RGB, attr Attr) {
	// Check what changed
	fgChanged := !o.lastValid || fg != o.lastFg || (attr&AttrFg256) != (o.lastAttr&AttrFg256)
	bgChanged := !o.lastValid || bg != o.lastBg || (attr&AttrBg256) != (o.lastAttr&AttrBg256)
	styleAttr := attr & AttrStyle
	lastStyleAttr := o.lastAttr & AttrStyle
	attrChanged := !o.lastValid || styleAttr != lastStyleAttr

	if !fgChanged && !bgChanged && !attrChanged {
		return
	}

	// If attributes changed, must reset first
	if attrChanged {
		w.Write(csi) // \x1b[
		first := true

		// Reset
		w.WriteByte('0')
		first = false

		// Style attributes
		if styleAttr&AttrBold != 0 {
			if !first {
				w.WriteByte(';')
			}
			w.WriteByte('1')
			first = false
		}
		if styleAttr&AttrDim != 0 {
			if !first {
				w.WriteByte(';')
			}
			w.WriteByte('2')
			first = false
		}
		if styleAttr&AttrItalic != 0 {
			if !first {
				w.WriteByte(';')
			}
			w.WriteByte('3')
			first = false
		}
		if styleAttr&AttrUnderline != 0 {
			if !first {
				w.WriteByte(';')
			}
			w.WriteByte('4')
			first = false
		}
		if styleAttr&AttrBlink != 0 {
			if !first {
				w.WriteByte(';')
			}
			w.WriteByte('5')
			first = false
		}
		if styleAttr&AttrReverse != 0 {
			if !first {
				w.WriteByte(';')
			}
			w.WriteByte('7')
			first = false
		}

		o.writeFgInline(w, fg, attr)
		o.writeBgInline(w, bg, attr)
		w.WriteByte('m')
	} else {
		// Only colors changed, emit minimal sequence
		if fgChanged && bgChanged {
			w.Write(csi)
			o.writeFgInline(w, fg, attr)
			o.writeBgInline(w, bg, attr)
			w.WriteByte('m')
		} else if fgChanged {
			o.writeFgFull(w, fg, attr)
		} else if bgChanged {
			o.writeBgFull(w, bg, attr)
		}
	}

	o.lastFg = fg
	o.lastBg = bg
	o.lastAttr = attr
	o.lastValid = true
}

// writeFgInline writes fg color parameters (no CSI prefix, no 'm' suffix)
func (o *outputBuffer) writeFgInline(w *bufio.Writer, fg RGB, attr Attr) {
	w.WriteByte(';')
	if attr&AttrFg256 != 0 {
		// 256-color: 38;5;N
		w.Write([]byte("38;5;"))
		writeInt(w, int(fg.R))
	} else if o.colorMode == ColorModeTrueColor {
		// True color: 38;2;R;G;B
		w.Write([]byte("38;2;"))
		writeInt(w, int(fg.R))
		w.WriteByte(';')
		writeInt(w, int(fg.G))
		w.WriteByte(';')
		writeInt(w, int(fg.B))
	} else {
		// Fallback 256: 38;5;N
		w.Write([]byte("38;5;"))
		writeInt(w, int(RGBTo256(fg)))
	}
}

// writeBgInline writes bg color parameters (no CSI prefix, no 'm' suffix)
func (o *outputBuffer) writeBgInline(w *bufio.Writer, bg RGB, attr Attr) {
	w.WriteByte(';')
	if attr&AttrBg256 != 0 {
		// 256-color: 48;5;N
		w.Write([]byte("48;5;"))
		writeInt(w, int(bg.R))
	} else if o.colorMode == ColorModeTrueColor {
		// True color: 48;2;R;G;B
		w.Write([]byte("48;2;"))
		writeInt(w, int(bg.R))
		w.WriteByte(';')
		writeInt(w, int(bg.G))
		w.WriteByte(';')
		writeInt(w, int(bg.B))
	} else {
		// Fallback 256: 48;5;N
		w.Write([]byte("48;5;"))
		writeInt(w, int(RGBTo256(bg)))
	}
}

// writeFgFull writes complete fg color sequence
func (o *outputBuffer) writeFgFull(w *bufio.Writer, fg RGB, attr Attr) {
	if attr&AttrFg256 != 0 {
		w.Write(csiFg256)
		writeInt(w, int(fg.R))
		w.WriteByte('m')
	} else if o.colorMode == ColorModeTrueColor {
		w.Write(csiFgRGB)
		writeInt(w, int(fg.R))
		w.WriteByte(';')
		writeInt(w, int(fg.G))
		w.WriteByte(';')
		writeInt(w, int(fg.B))
		w.WriteByte('m')
	} else {
		w.Write(csiFg256)
		writeInt(w, int(RGBTo256(fg)))
		w.WriteByte('m')
	}
}

// writeBgFull writes complete bg color sequence
func (o *outputBuffer) writeBgFull(w *bufio.Writer, bg RGB, attr Attr) {
	if attr&AttrBg256 != 0 {
		w.Write(csiBg256)
		writeInt(w, int(bg.R))
		w.WriteByte('m')
	} else if o.colorMode == ColorModeTrueColor {
		w.Write(csiBgRGB)
		writeInt(w, int(bg.R))
		w.WriteByte(';')
		writeInt(w, int(bg.G))
		w.WriteByte(';')
		writeInt(w, int(bg.B))
		w.WriteByte('m')
	} else {
		w.Write(csiBg256)
		writeInt(w, int(RGBTo256(bg)))
		w.WriteByte('m')
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

// clear writes a clear screen with specified background
func (o *outputBuffer) clear(bg RGB) {
	w := o.writer
	w.Write(csiSGR0)
	o.writeBgFull(w, bg, 0)
	w.Write(csiClear)

	o.lastValid = false
	o.cursorValid = false
	w.Flush()

	for i := range o.front {
		o.front[i] = Cell{Rune: ' ', Bg: bg}
	}
}

// invalidateCursor marks cursor position as unknown
func (o *outputBuffer) invalidateCursor() {
	o.cursorValid = false
}