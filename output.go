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

func newOutputBuffer(backend Backend, colorMode ColorMode) *outputBuffer {
	// Use 128KB buffer for minimal calls to backend
	adapter := writerAdapter{b: backend}
	return &outputBuffer{
		writer:    bufio.NewWriterSize(adapter, 131072),
		colorMode: colorMode,
	}
}

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

// cellEqual compares the fields most likely to change first
func cellEqual(a, b Cell) bool {
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

		rowEnd := width
		for rowEnd > 0 && cellEqual(cells[rowStart+rowEnd-1], o.front[rowStart+rowEnd-1]) {
			rowEnd--
		}
		if rowEnd == 0 {
			continue
		}

		x := 0
		for x < rowEnd {
			idx := rowStart + x

			if cellEqual(cells[idx], o.front[idx]) {
				x++
				continue
			}

			segStart := x
			segEnd := x + 1

			// A gap of up to 3 unchanged cells is rewritten, about the bytes of a cursor move
			for segEnd < rowEnd {
				gapStart := segEnd
				for gapStart < rowEnd && cellEqual(cells[rowStart+gapStart], o.front[rowStart+gapStart]) {
					gapStart++
				}
				gapSize := gapStart - segEnd

				if gapSize == 0 {
					for segEnd < rowEnd && !cellEqual(cells[rowStart+segEnd], o.front[rowStart+segEnd]) {
						segEnd++
					}
					continue
				}

				if gapSize > 3 {
					break
				}

				// Only a gap in the segment's style: an SGR inside it costs more than the move
				gapCompatible := true
				refCell := cells[rowStart+segEnd-1]

				for k := 0; k < gapSize; k++ {
					gCell := cells[rowStart+segEnd+k]
					if gCell.Fg != refCell.Fg || gCell.Bg != refCell.Bg || gCell.Attrs != refCell.Attrs {
						gapCompatible = false
						break
					}
				}

				if !gapCompatible {
					break
				}

				if gapStart >= rowEnd {
					break
				}

				segEnd = gapStart
				for segEnd < rowEnd && !cellEqual(cells[rowStart+segEnd], o.front[rowStart+segEnd]) {
					segEnd++
				}
			}

			o.moveCursorTo(w, segStart, y)

			for sx := segStart; sx < segEnd; sx++ {
				cidx := rowStart + sx
				c := cells[cidx]

				o.writeStyleCoalesced(w, c.Fg, c.Bg, c.Attrs)
				if r := c.Rune; r >= ' ' && r < 0x7f {
					w.WriteByte(byte(r))
				} else {
					w.WriteRune(oneColumn(r))
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

// oneColumn is r as a cell draws it: one glyph in one column, where the
// terminal draws ambiguous-width runes (box drawing, U+FFFD) narrow
func oneColumn(r rune) rune {
	if uint32(r) < 0x10000 && plain[r>>6]&(1<<(r&63)) != 0 {
		return r
	}
	if !utf8.ValidRune(r) {
		return ' '
	}
	for _, k := range drawnAs {
		if unicode.In(r, k.runes...) {
			return k.as
		}
	}
	return r
}

// drawnAs is the glyph a cell draws for each kind of rune that would not take
// one column: a space for one a terminal acts on, hides or reorders text by
// (a control, a format character, a line separator), U+FFFD for one it draws
// in none or two or joins to a neighbour (a mark, a wide or emoji rune)
var drawnAs = [...]struct {
	as    rune
	runes []*unicode.RangeTable
}{
	{' ', []*unicode.RangeTable{unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp}},
	{utf8.RuneError, []*unicode.RangeTable{wideOrJoining, unicode.Mn, unicode.Me, unicode.Mc}},
}

// plain has a bit set for each BMP rune a cell draws as itself, so most text
// skips drawnAs's searches
var plain = func() (p [0x10000 >> 6]uint64) {
	for i := range p {
		if i < 0xD800>>6 || i >= 0xE000>>6 {
			p[i] = ^uint64(0)
		}
	}
	for _, k := range drawnAs {
		for _, t := range k.runes {
			for _, r := range t.R16 {
				for c := int(r.Lo); c <= int(r.Hi); c += int(r.Stride) {
					p[c>>6] &^= 1 << (c & 63)
				}
			}
		}
	}
	return p
}()

// wideOrJoining is what a terminal draws in two columns, East Asian wide and
// fullwidth and emoji presentation as of Unicode 17, and the letters a
// grapheme cluster joins to a neighbour (Hangul jamo, Prepend, SpacingMark and
// Extend ones); a range spans the unassigned code points between its runes
var wideOrJoining = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x0D4E, 0x0D4E, 1}, {0x0E33, 0x0E33, 1}, {0x0EB3, 0x0EB3, 1},
		{0x1100, 0x11FF, 1}, {0x231A, 0x231B, 1}, {0x2329, 0x232A, 1}, {0x23E9, 0x23EC, 1},
		{0x23F0, 0x23F0, 1}, {0x23F3, 0x23F3, 1}, {0x25FD, 0x25FE, 1}, {0x2614, 0x2615, 1},
		{0x2630, 0x2637, 1}, {0x2648, 0x2653, 1}, {0x267F, 0x267F, 1}, {0x268A, 0x268F, 1},
		{0x2693, 0x2693, 1}, {0x26A1, 0x26A1, 1}, {0x26AA, 0x26AB, 1}, {0x26BD, 0x26BE, 1},
		{0x26C4, 0x26C5, 1}, {0x26CE, 0x26CE, 1}, {0x26D4, 0x26D4, 1}, {0x26EA, 0x26EA, 1},
		{0x26F2, 0x26F3, 1}, {0x26F5, 0x26F5, 1}, {0x26FA, 0x26FA, 1}, {0x26FD, 0x26FD, 1},
		{0x2705, 0x2705, 1}, {0x270A, 0x270B, 1}, {0x2728, 0x2728, 1}, {0x274C, 0x274C, 1},
		{0x274E, 0x274E, 1}, {0x2753, 0x2755, 1}, {0x2757, 0x2757, 1}, {0x2795, 0x2797, 1},
		{0x27B0, 0x27B0, 1}, {0x27BF, 0x27BF, 1}, {0x2B1B, 0x2B1C, 1}, {0x2B50, 0x2B50, 1},
		{0x2B55, 0x2B55, 1}, {0x2E80, 0x303E, 1}, {0x3041, 0x3247, 1}, {0x3250, 0xA4C6, 1},
		{0xA960, 0xA97C, 1}, {0xAC00, 0xD7FF, 1}, {0xF900, 0xFAFF, 1}, {0xFE10, 0xFE19, 1},
		{0xFE30, 0xFE6B, 1}, {0xFF01, 0xFF60, 1}, {0xFF9E, 0xFF9F, 1}, {0xFFE0, 0xFFE6, 1},
	},
	R32: []unicode.Range32{
		{0x111C2, 0x111C3, 1}, {0x113D1, 0x113D1, 1}, {0x1193F, 0x1193F, 1}, {0x11941, 0x11941, 1},
		{0x11A84, 0x11A89, 1}, {0x11D46, 0x11D46, 1}, {0x11F02, 0x11F02, 1},
		{0x16FE0, 0x1B2FB, 1}, {0x1D300, 0x1D376, 1}, {0x1F004, 0x1F004, 1}, {0x1F0CF, 0x1F0CF, 1},
		{0x1F18E, 0x1F18E, 1}, {0x1F191, 0x1F19A, 1}, {0x1F1E6, 0x1F320, 1}, {0x1F32D, 0x1F335, 1},
		{0x1F337, 0x1F37C, 1}, {0x1F37E, 0x1F393, 1}, {0x1F3A0, 0x1F3CA, 1}, {0x1F3CF, 0x1F3D3, 1},
		{0x1F3E0, 0x1F3F0, 1}, {0x1F3F4, 0x1F3F4, 1}, {0x1F3F8, 0x1F43E, 1}, {0x1F440, 0x1F440, 1},
		{0x1F442, 0x1F4FC, 1}, {0x1F4FF, 0x1F53D, 1}, {0x1F54B, 0x1F54E, 1}, {0x1F550, 0x1F567, 1},
		{0x1F57A, 0x1F57A, 1}, {0x1F595, 0x1F596, 1}, {0x1F5A4, 0x1F5A4, 1}, {0x1F5FB, 0x1F64F, 1},
		{0x1F680, 0x1F6C5, 1}, {0x1F6CC, 0x1F6CC, 1}, {0x1F6D0, 0x1F6D2, 1}, {0x1F6D5, 0x1F6DF, 1},
		{0x1F6EB, 0x1F6EC, 1}, {0x1F6F4, 0x1F6FC, 1}, {0x1F7E0, 0x1F7F0, 1}, {0x1F90C, 0x1F93A, 1},
		{0x1F93C, 0x1F945, 1}, {0x1F947, 0x1F9FF, 1}, {0x1FA70, 0x1FAF8, 1}, {0x20000, 0x3FFFF, 1},
	},
}

func cursorForwardCost(n int) int {
	if n == 1 {
		return 3 // \x1b[C
	}
	return 3 + digitCount(n) // \x1b[nC
}

func cursorAbsoluteCost(x, y int) int {
	// \x1b[row;colH = 2 + digits(row) + 1 + digits(col) + 1
	return 4 + digitCount(y+1) + digitCount(x+1)
}

// digitCount caps at 4: no terminal is 10000 cells across
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

func (o *outputBuffer) invalidateCursor() {
	o.cursorValid = false
}
