package terminal

import (
	"bytes"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/lixenwraith/color"
)

// outputBuffer renders double-buffered terminal output with diffing. It only
// encodes: what an operation renders collects in buf, and termImpl hands it to
// the goroutine that writes it.
type outputBuffer struct {
	front     []Cell
	width     int
	height    int
	colorMode ColorMode
	buf       []byte

	cursorX     int
	cursorY     int
	cursorValid bool
	cursorShown bool // the caller shows the cursor; a frame leaves it as it is

	// Style state for coalescing, and lastKey, the styleKey of the cell it was
	// last given: a cell in that style needs no SGR
	lastFg    color.RGB
	lastBg    color.RGB
	lastAttr  Attr
	lastKey   uint64
	lastValid bool
}

// writerAdapter adapts Backend to io.Writer
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

func newOutputBuffer(colorMode ColorMode) *outputBuffer {
	return &outputBuffer{colorMode: colorMode}
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

// Cell has no padding, so a row compares as the bytes it occupies; the assertion
// fails to compile should a field change leave padding behind
var _ [0]struct{} = [unsafe.Sizeof(Cell{}) - unsafe.Sizeof(rune(0)) - 2*unsafe.Sizeof(color.RGB{}) - unsafe.Sizeof(Attr(0))]struct{}{}

// cellsEqual reports whether two runs of cells hold the same cells, as one memcmp
func cellsEqual(a, b []Cell) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	n := len(a) * int(unsafe.Sizeof(Cell{}))
	return bytes.Equal(unsafe.Slice((*byte)(unsafe.Pointer(&a[0])), n), unsafe.Slice((*byte)(unsafe.Pointer(&b[0])), n))
}

// cellWords is cells as the three words each holds, the rune, then its colors
// and attributes: two cells compare in one branch, and two styles in one compare
func cellWords(c []Cell) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(unsafe.SliceData(c))), 3*len(c))
}

// differs reports whether cell i of a and b, as cellWords, differ
func differs(a, b []uint32, i int) bool {
	a, b = a[3*i:3*i+3], b[3*i:3*i+3]
	return (a[0]^b[0])|(a[1]^b[1])|(a[2]^b[2]) != 0
}

// styleKey is cell i's colors and attributes, of cellWords, as one word
func styleKey(w []uint32, i int) uint64 {
	w = w[3*i+1 : 3*i+3]
	return uint64(w[0]) | uint64(w[1])<<32
}

// diffChunk is the cells one memcmp settles while a row's changes, or the end of
// an unchanged run, are sought: few enough that a changed chunk costs little to
// search again cell by cell
const diffChunk = 32

// firstChange is the first cell from on where a and b differ, len(a) when none does
func firstChange(a, b []Cell, from int) int {
	i := from
	for i+diffChunk <= len(a) && cellsEqual(a[i:i+diffChunk], b[i:i+diffChunk]) {
		i += diffChunk
	}
	aw, bw := cellWords(a), cellWords(b)
	for i < len(a) && !differs(aw, bw, i) {
		i++
	}
	return i
}

// lastChange is one past the last cell from on where a and b differ, from when none does
func lastChange(a, b []Cell, from int) int {
	j := len(a)
	for j-diffChunk >= from && cellsEqual(a[j-diffChunk:j], b[j-diffChunk:j]) {
		j -= diffChunk
	}
	aw, bw := cellWords(a), cellWords(b)
	for j > from && !differs(aw, bw, j-1) {
		j--
	}
	return j
}

// flush renders the back buffer into buf, diffing against front buffer
func (o *outputBuffer) flush(cells []Cell, width, height int) {
	if width != o.width || height != o.height {
		o.resize(width, height)
	}

	expectedSize := width * height
	if len(cells) < expectedSize {
		return
	}

	b := o.buf
	start := len(b)
	b = append(b, csiSyncBegin...)
	// A host that shows the cursor on its own (a console host repainting, a resize)
	// would show it where the last cell went: a hidden one is hidden again at each end
	if !o.cursorShown {
		b = append(b, csiCursorHide...)
	}
	body := len(b)

	for y := 0; y < height; y++ {
		// Memcmp finds the row's first and last changed cells, so an unchanged row is
		// one pass of compares; the cells between them are drawn, and the front
		// buffer takes them as one run
		row, front := cells[y*width:(y+1)*width], o.front[y*width:(y+1)*width]
		x := firstChange(row, front, 0)
		if x == width {
			continue
		}
		end := lastChange(row, front, x+1)
		b = o.span(b, row, front, x, end, y)
		copy(front[x:end], row[x:end])
	}

	// A frame that changed no cell writes nothing, the reset and markers included
	if len(b) == body {
		b = b[:start]
	} else {
		b = append(b, csiSGR0...)
		if !o.cursorShown {
			b = append(b, csiCursorHide...)
		}
		b = append(b, csiSyncEnd...)
	}
	o.buf = b
	o.lastValid = false
}

// span draws row y's changed cells from x to end, x and end-1 among them. A gap
// of up to 3 unchanged cells in the style before it is drawn again, about the
// bytes of a cursor move; a longer one, or one an SGR would interrupt, is moved over.
func (o *outputBuffer) span(b []byte, row, front []Cell, x, end, y int) []byte {
	rw, fw := cellWords(row), cellWords(front)
	for x < end {
		segEnd, next := x+1, end
		for segEnd < end {
			if differs(rw, fw, segEnd) {
				segEnd++
				continue
			}
			g := segEnd + 1
			for g < end && g-segEnd <= 3 && !differs(rw, fw, g) {
				g++
			}
			if g-segEnd > 3 {
				next = firstChange(row[:end], front[:end], g)
				break
			}
			if !sameStyle(rw, segEnd-1, g) {
				next = g
				break
			}
			segEnd = g
		}

		b = o.moveCursorTo(b, x, y)
		for ; x < segEnd; x++ {
			if k := styleKey(rw, x); !o.lastValid || k != o.lastKey {
				c := &row[x]
				b = o.appendStyle(b, c.Fg, c.Bg, c.Attrs)
				o.lastKey = k
			}
			if r := row[x].Rune; r >= ' ' && r < 0x7f {
				b = append(b, byte(r))
			} else {
				b = utf8.AppendRune(b, oneColumn(r))
			}
		}
		o.cursorX = segEnd
		x = next
	}
	return b
}

// sameStyle reports whether the cells after ref and before j, of cellWords w,
// are in cell ref's style
func sameStyle(w []uint32, ref, j int) bool {
	k := styleKey(w, ref)
	for i := ref + 1; i < j; i++ {
		if styleKey(w, i) != k {
			return false
		}
	}
	return true
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
func (o *outputBuffer) moveCursorTo(b []byte, x, y int) []byte {
	if o.cursorValid && o.cursorX == x && o.cursorY == y {
		return b
	}
	if o.cursorValid && o.cursorY == y && x > o.cursorX && cursorForwardCost(x-o.cursorX) < cursorAbsoluteCost(x, y) {
		b = appendCursorForward(b, x-o.cursorX)
	} else {
		b = appendCursorPos(b, x, y)
	}
	o.cursorX = x
	o.cursorY = y
	o.cursorValid = true
	return b
}

// appendStyle emits a single combined SGR sequence when style changes
func (o *outputBuffer) appendStyle(b []byte, fg, bg color.RGB, attr Attr) []byte {
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
		return b
	}

	b = append(b, csi...)
	// If attributes changed, must reset first, then restate both colors
	if attrChanged {
		b = append(b, '0')
		for _, a := range attrCodes {
			if styleAttr&a.bit != 0 {
				b = append(b, ';', a.code)
			}
		}
		fgChanged, bgChanged = true, true
	}
	if fgChanged {
		b = o.appendColor(b, fg, attr&AttrFg256 != 0, attr&AttrFgDefault != 0, 30, attrChanged)
	}
	if bgChanged {
		b = o.appendColor(b, bg, attr&AttrBg256 != 0, attr&AttrBgDefault != 0, 40, attrChanged || fgChanged)
	}

	o.lastFg = fg
	o.lastBg = bg
	o.lastAttr = attr
	o.lastValid = true
	return append(b, 'm')
}

// attrCodes pairs each style bit with its SGR parameter, in emission order
var attrCodes = [...]struct {
	bit  Attr
	code byte
}{{AttrBold, '1'}, {AttrDim, '2'}, {AttrItalic, '3'}, {AttrUnderline, '4'}, {AttrBlink, '5'}, {AttrReverse, '7'}}

// appendColor appends one color's SGR parameters, after a ';' when sep, for
// base 30 (foreground) or 40 (background): the terminal's own, a palette
// index, or RGB, as the color mode allows. ColorModeNone appends nothing.
func (o *outputBuffer) appendColor(b []byte, c color.RGB, palette, deflt bool, base uint8, sep bool) []byte {
	if o.colorMode == ColorModeNone {
		return b
	}
	if sep {
		b = append(b, ';')
	}
	switch {
	case deflt:
		return appendU8(b, base+9)
	case o.colorMode == ColorMode16:
		i := color.RGBTo16(c)
		if palette {
			i = paletteTo16(c.R)
		}
		if i >= 8 {
			base += 60 - 8 // bright: 90-97, 100-107
		}
		return appendU8(b, base+i)
	case palette:
		b = append(appendU8(b, base+8), ";5;"...)
		return appendU8(b, c.R)
	case o.colorMode == ColorModeTrueColor:
		b = append(appendU8(b, base+8), ";2;"...)
		b = append(appendU8(b, c.R), ';')
		b = append(appendU8(b, c.G), ';')
		return appendU8(b, c.B)
	}
	b = append(appendU8(b, base+8), ";5;"...)
	return appendU8(b, RGBTo256(c))
}

func (o *outputBuffer) forceFullRedraw() {
	for i := range o.front {
		o.front[i] = Cell{Rune: 0}
	}
	o.lastValid = false
	o.cursorValid = false
}

// clear renders a clear screen with specified background, attr's
// AttrBgDefault or AttrBg256 saying how to read it
func (o *outputBuffer) clear(bg color.RGB, attr Attr) {
	b := append(o.buf, csiSGR0...)
	b = o.appendColor(append(b, csi...), bg, attr&AttrBg256 != 0, attr&AttrBgDefault != 0, 40, false)
	o.buf = append(append(b, 'm'), csiClear...)

	o.lastValid = false
	o.cursorValid = false

	for i := range o.front {
		o.front[i] = Cell{Rune: ' ', Bg: bg, Attrs: attr & AttrBgColor}
	}
}

func (o *outputBuffer) invalidateCursor() {
	o.cursorValid = false
}
