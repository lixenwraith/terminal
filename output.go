package terminal

import (
	"math/bits"
	"runtime"
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

	diff []uint64 // a row's changed cells, as ChangedCells marks them

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

// Cell has no padding: it is the three words cellWords and the ChangedCells
// kernels read, and the assertion fails to compile should that change
var _ [0]struct{} = [unsafe.Sizeof(Cell{}) - unsafe.Sizeof(rune(0)) - 2*unsafe.Sizeof(color.RGB{}) - unsafe.Sizeof(Attr(0))]struct{}{}

// cellWords is cells as the three words each holds, the rune, then its colors
// and attributes: two cells compare in one branch, and two styles in one compare
func cellWords(c []Cell) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(unsafe.SliceData(c))), 3*len(c))
}

// styleKey is cell i's colors and attributes, of cellWords, as one word
func styleKey(w []uint32, i int) uint64 {
	w = w[3*i+1 : 3*i+3]
	return uint64(w[0]) | uint64(w[1])<<32
}

// ChangedCells returns, in dst's storage, a bit for each cell where a and b
// differ: bit i&63 of word i>>6 for cell i of the first min(len(a), len(b)),
// the bits past them clear. A frame's changed cells are the bits set against
// the frame before; built with GOEXPERIMENT=simd, cells compare as vectors.
func ChangedCells(dst []uint64, a, b []Cell) []uint64 {
	n := min(len(a), len(b))
	words := (n + 63) >> 6
	if cap(dst) < words {
		dst = make([]uint64, words)
	}
	dst = dst[:words]
	changedBits(dst, cellWords(a[:n]), cellWords(b[:n]), n)
	return dst
}

// skipEqual is whether a block that one memcmp finds unchanged is skipped:
// everywhere but wasm, whose memcmp goes a byte at a time
const skipEqual = runtime.GOARCH != "wasm"

// changedWords sets m's words from w on as changedBits does, in plain Go: a
// block of 64 cells, then of 16, that one memcmp finds unchanged is skipped
func changedWords(m []uint64, a, b []uint32, n, w int) {
	for ; w < len(m); w++ {
		lo, hi := w<<6, min(w<<6+64, n)
		if skipEqual && hi-lo == 64 && *(*[192]uint32)(a[3*lo:]) == *(*[192]uint32)(b[3*lo:]) {
			m[w] = 0
			continue
		}
		var bits uint64
		s := lo
		for ; s+16 <= hi; s += 16 {
			if p, q := (*[48]uint32)(a[3*s:]), (*[48]uint32)(b[3*s:]); !skipEqual || *p != *q {
				bits |= uint64(cellBits16(p, q)) << (s - lo)
			}
		}
		for i := s; i < hi; i++ {
			p, q := a[3*i:3*i+3:3*i+3], b[3*i:3*i+3:3*i+3]
			d := (p[0] ^ q[0]) | (p[1] ^ q[1]) | (p[2] ^ q[2])
			bits |= uint64((d|-d)>>31) << (i - lo)
		}
		m[w] = bits
	}
}

// cellBits16 is a bit for each of 16 cells, as cellWords, that differs
func cellBits16(p, q *[48]uint32) uint16 {
	var bits uint16
	for i := range 16 {
		d := (p[3*i] ^ q[3*i]) | (p[3*i+1] ^ q[3*i+1]) | (p[3*i+2] ^ q[3*i+2])
		bits |= uint16((d|-d)>>31) << i
	}
	return bits
}

// nextSet is the first cell from p before end that m marks, end when none is
func nextSet(m []uint64, p, end int) int {
	for p < end {
		if w := m[p>>6] >> (p & 63); w != 0 {
			return min(p+bits.TrailingZeros64(w), end)
		}
		p = (p | 63) + 1
	}
	return end
}

// nextClear is the first cell from p before end that m leaves clear, end when
// none is
func nextClear(m []uint64, p, end int) int {
	for p < end {
		if w := ^m[p>>6] >> (p & 63); w != 0 {
			return min(p+bits.TrailingZeros64(w), end)
		}
		p = (p | 63) + 1
	}
	return end
}

// lastSet is one past the last cell m marks, 0 when it marks none
func lastSet(m []uint64) int {
	for k := len(m) - 1; k >= 0; k-- {
		if m[k] != 0 {
			return k<<6 + 64 - bits.LeadingZeros64(m[k])
		}
	}
	return 0
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
		// The row's changed cells as bits: those between the first and the last
		// are drawn, and the front buffer takes them as one run
		row, front := cells[y*width:(y+1)*width], o.front[y*width:(y+1)*width]
		o.diff = ChangedCells(o.diff, row, front)
		x := nextSet(o.diff, 0, width)
		if x == width {
			continue
		}
		end := lastSet(o.diff)
		b = o.span(b, row, o.diff, x, end, y)
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

// span draws row y's changed cells, the ones m marks, from x to end, x and end-1
// among them. A gap of up to 3 unchanged cells in the style before it is drawn
// again, about the bytes of a cursor move; a longer one, or one an SGR would
// interrupt, is moved over.
func (o *outputBuffer) span(b []byte, row []Cell, m []uint64, x, end, y int) []byte {
	rw := cellWords(row)
	for x < end {
		segEnd, next := end, end
		for p := x; ; {
			run := nextClear(m, p, end)
			if run == end {
				break
			}
			g := nextSet(m, run, end)
			if g-run > 3 || !sameStyle(rw, run-1, g) {
				segEnd, next = run, g
				break
			}
			p = g
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

// OneColumn reports whether a cell draws r as itself, in one column; Flush
// draws any other rune as a space or U+FFFD
func OneColumn(r rune) bool { return oneColumn(r) == r }

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
		b = o.appendColor(b, fg, attr, 30, attrChanged)
	}
	if bgChanged {
		b = o.appendColor(b, bg, attr, 40, attrChanged || fgChanged)
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
// base 30 (foreground) or 40 (background), as attr says to read it: the
// terminal's own, a palette index, or RGB, as the color mode allows.
// ColorModeNone appends nothing.
func (o *outputBuffer) appendColor(b []byte, c color.RGB, attr Attr, base uint8, sep bool) []byte {
	if o.colorMode == ColorModeNone {
		return b
	}
	palette, deflt := attr&AttrFg256 != 0, attr&AttrFgDefault != 0
	if base == 40 {
		palette, deflt = attr&AttrBg256 != 0, attr&AttrBgDefault != 0
	}
	if sep {
		b = append(b, ';')
	}
	switch {
	case deflt:
		return appendU8(b, base+9)
	case o.colorMode == ColorMode16 && palette:
		return appendANSI16(b, base, color.Xterm256To16(c.R))
	case o.colorMode == ColorMode16:
		return appendANSI16(b, base, color.RGBTo16(c))
	case palette && c.R < 16 && (c.R >= 8 || base == 40 || attr&AttrBold == 0):
		// The theme's sixteen in fewer bytes; a bold foreground below 8 keeps
		// 38;5, as a terminal drawing bold bright may brighten 30-37 alone
		return appendANSI16(b, base, c.R)
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

// appendANSI16 appends ANSI 16 index i as its SGR code over base: 30-37 and
// 90-97, or 40-47 and 100-107
func appendANSI16(b []byte, base, i uint8) []byte {
	if i >= 8 {
		base += 60 - 8
	}
	return appendU8(b, base+i)
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
	b = o.appendColor(append(b, csi...), bg, attr, 40, false)
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
