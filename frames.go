package terminal

import (
	"encoding/binary"
	"math/bits"
	"slices"
	"sync"
	"time"

	"github.com/lixenwraith/color"
)

// FrameEncoder appends to dst a frame of cols by rows cells as its drawer reads
// one: the cells changed marks, as ChangedCells does, or every cell for a nil
// changed
type FrameEncoder func(dst []byte, cells []Cell, cols, rows int, changed []uint64) []byte

// Frames hands the frames a Surface receives to a drawer in another runtime, a Java
// view's thread or a browser page: Flush keeps the newest, Take encodes the cells
// changed since the frame the drawer holds, and Blocked is how long the oldest frame
// not taken has waited. Any goroutine may call any method.
type Frames struct {
	mu     sync.Mutex
	encode FrameEncoder
	next   []Cell    // the newest frame flushed
	fw, fh int       // its size
	shown  []Cell    // the frame the drawer holds
	fresh  bool      // next is not taken yet
	whole  bool      // the next take is every cell
	since  time.Time // when the oldest frame not taken was flushed
	out    []byte    // the encoded frame, reused between takes
	diff   []uint64  // the cells next changes from shown, as ChangedCells marks them
}

// NewFrames takes frames for a drawer that reads them as encode writes them, nil
// for RuneFrame
func NewFrames(encode FrameEncoder) *Frames { return &Frames{encode: encode} }

// SetEncoder is for a drawer whose glyphs changed, which voids the frame it holds:
// the newest frame is its to take again, every cell as encode writes it
func (f *Frames) SetEncoder(encode FrameEncoder) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.encode, f.whole = encode, true
	if len(f.shown) > 0 && !f.fresh {
		f.fresh, f.since = true, time.Now()
	}
}

// ColorMode is truecolor: a drawer paints RGB
func (f *Frames) ColorMode() ColorMode { return ColorModeTrueColor }

// Flush keeps a frame of width by height cells as the newest, replacing one not yet
// taken; a frame short of its own size is dropped, as a terminal drops one
func (f *Frames) Flush(cells []Cell, width, height int) {
	n := width * height
	if width < 0 || height < 0 || len(cells) < n {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if width != f.fw || height != f.fh {
		f.fw, f.fh, f.whole = width, height, true
	}
	f.next = append(f.next[:0], cells[:n]...)
	if !f.fresh {
		f.fresh, f.since = true, time.Now()
	}
}

// Sync makes the next take every cell
func (f *Frames) Sync() {
	f.mu.Lock()
	f.whole = true
	f.mu.Unlock()
}

// Blocked is how long the oldest frame the drawer has not taken has waited
func (f *Frames) Blocked() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.since.IsZero() {
		return 0
	}
	return time.Since(f.since)
}

// Take encodes the newest frame for the drawer, valid until the next take: every
// cell after a resize, a Sync or a SetEncoder, and nil when no cell changed since
// the frame the drawer holds
func (f *Frames) Take() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.fresh {
		return nil
	}
	whole := f.whole || len(f.shown) != len(f.next)
	f.fresh, f.whole, f.since = false, false, time.Time{}
	var changed []uint64
	if !whole {
		f.diff = ChangedCells(f.diff, f.next, f.shown)
		if !slices.ContainsFunc(f.diff, func(w uint64) bool { return w != 0 }) {
			return nil
		}
		changed = f.diff
	}
	encode := f.encode
	if encode == nil {
		encode = RuneFrame
	}
	f.out = encode(f.out[:0], f.next, f.fw, f.fh, changed)
	f.shown = append(f.shown[:0], f.next...)
	return f.out
}

// CellFrame appends a frame as little-endian 32-bit words: cols, rows and the
// count of records, then a record for each cell changed marks, or for every cell
// when changed is nil: its index, the id of its rune (a drawer's name for the
// glyph), and its foreground and background as ARGB
func CellFrame(dst []byte, cells []Cell, cols, rows int, changed []uint64, id func(rune) uint32) []byte {
	at := len(dst)
	dst = appendWords(dst, uint32(cols), uint32(rows), 0)
	n := 0
	if changed == nil {
		for i, c := range cells {
			dst = appendCell(dst, i, c, id)
		}
		n = len(cells)
	}
	for w, word := range changed {
		for ; word != 0; word &= word - 1 {
			i := w<<6 | bits.TrailingZeros64(word)
			dst = appendCell(dst, i, cells[i], id)
			n++
		}
	}
	binary.LittleEndian.PutUint32(dst[at+8:], uint32(n))
	return dst
}

// RuneFrame is a CellFrame whose ids are the runes a Terminal's Flush would draw,
// for a drawer that keeps glyphs of its own
func RuneFrame(dst []byte, cells []Cell, cols, rows int, changed []uint64) []byte {
	return CellFrame(dst, cells, cols, rows, changed, func(r rune) uint32 { return uint32(oneColumn(r)) })
}

func appendCell(dst []byte, i int, c Cell, id func(rune) uint32) []byte {
	fg, bg := CellColors(c)
	return appendWords(dst, uint32(i), id(c.Rune), argb(fg), argb(bg))
}

func argb(c color.RGB) uint32 {
	return 0xff000000 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}

func appendWords(dst []byte, words ...uint32) []byte {
	for _, w := range words {
		dst = binary.LittleEndian.AppendUint32(dst, w)
	}
	return dst
}

// Default colors for a cell that asks for the terminal's own
var (
	defaultFg = color.RGB{R: 0xc0, G: 0xc0, B: 0xc0}
	defaultBg = color.RGB{}
)

// CellColors is a cell's foreground and background as a drawer paints them: reverse
// swaps the two and dim halves the foreground; no other style reaches a drawer
func CellColors(c Cell) (fg, bg color.RGB) {
	fg, bg = cellColor(c.Fg, c.Attrs&AttrFg256 != 0, c.Attrs&AttrFgDefault != 0, defaultFg),
		cellColor(c.Bg, c.Attrs&AttrBg256 != 0, c.Attrs&AttrBgDefault != 0, defaultBg)
	if c.Attrs&AttrReverse != 0 {
		fg, bg = bg, fg
	}
	if c.Attrs&AttrDim != 0 {
		fg = color.Scale(fg, 0.5)
	}
	return fg, bg
}

// cellColor resolves one channel: the terminal's own, a palette index held in R,
// or the RGB as sent
func cellColor(c color.RGB, palette, own bool, def color.RGB) color.RGB {
	switch {
	case own:
		return def
	case palette:
		return color.Xterm256(c.R)
	}
	return c
}
