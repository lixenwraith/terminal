// Package glyph is a bitmap font of several cell sizes, the software rasterizer that
// draws a cell grid with one of them (the reference drawer of any graphical host and
// the fallback of every one) and what a drawer in another runtime paints from: a
// cell size as one texture, and frames encoded for it.
package glyph

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/lixenwraith/terminal"
)

// Font is one face at several cell sizes, its strikes, read where a font file's
// bytes lie: the README lays the file out
type Font struct {
	Name    string
	runes   []byte   // a little-endian word a glyph, ascending
	strikes []*Atlas // smallest first
}

// Atlas is one strike of a Font. A glyph's rows are whole bytes each, the leftmost
// column the first one's highest bit, so a cell is at most 32 pixels wide.
type Atlas struct {
	Width, Height int
	font          *Font
	stride        int    // bytes a row
	rows          []byte // every glyph's rows, in the order of the font's runes
}

const magic = "TGF1"

// Parse reads a font file, as tool/font-atlas writes it. The font keeps data and
// reads its glyphs from it, so the caller leaves it as it is.
func Parse(data []byte) (*Font, error) {
	le := binary.LittleEndian
	if len(data) < 12 || string(data[:4]) != magic {
		return nil, errors.New("font file: no " + magic + " header")
	}
	n, l, g := int(le.Uint16(data[4:])), int(le.Uint16(data[6:])), int64(le.Uint32(data[8:]))
	rest, named := data[12:], (l+3)&^3
	sized := func() error {
		return fmt.Errorf("font file: %d bytes are not the %d strikes of %d glyphs it counts", len(data), n, g)
	}
	// 64 bits wide, so counts a file made up cannot wrap
	if n < 1 || int64(len(rest)) < int64(named)+4*g+4*int64(n) {
		return nil, sized()
	}
	name, runes := rest[:named], rest[named:named+4*int(g)]
	rest = rest[len(name)+len(runes):]
	if !utf8.Valid(name[:l]) || string(name[l:]) != "\x00\x00\x00"[:named-l] {
		return nil, errors.New("font file: the name is not UTF-8 padded with zero bytes")
	}
	for i := 0; i < len(runes); i += 4 {
		if r := le.Uint32(runes[i:]); r > utf8.MaxRune || i > 0 && r <= le.Uint32(runes[i-4:]) {
			return nil, fmt.Errorf("font file: glyph %d is %#x, not a code point above the one before", i/4, r)
		}
	}
	f := &Font{Name: string(name[:l]), runes: runes, strikes: make([]*Atlas, 0, n)}
	for i := range n {
		if len(rest) < 4 {
			return nil, sized()
		}
		w, h := int(le.Uint16(rest)), int(le.Uint16(rest[2:]))
		if w < 1 || w > 32 || h < 1 || i > 0 && h <= f.strikes[i-1].Height {
			return nil, fmt.Errorf("font file: strike %d is %dx%d, not 1-32 wide and taller than the one before", i, w, h)
		}
		a := &Atlas{Width: w, Height: h, font: f, stride: (w + 7) / 8}
		size := g * int64(h*a.stride)
		if int64(len(rest)-4) < size {
			return nil, sized()
		}
		a.rows, rest = rest[4:4+int(size)], rest[4+int(size):]
		if spare := byte(0xff) >> (w & 7); w&7 != 0 {
			for at := a.stride - 1; at < len(a.rows); at += a.stride {
				if a.rows[at]&spare != 0 {
					return nil, fmt.Errorf("font file: strike %d sets a bit past its width in glyph %d", i, at/(h*a.stride))
				}
			}
		}
		f.strikes = append(f.strikes, a)
	}
	if len(rest) != 0 {
		return nil, sized()
	}
	return f, nil
}

// Strikes is the font's strikes, smallest first
func (f *Font) Strikes() []*Atlas { return f.strikes }

// Strike is the largest strike no taller than height pixels, the smallest when
// none is
func (f *Font) Strike(height int) *Atlas {
	strike := f.strikes[0]
	for _, a := range f.strikes[1:] {
		if a.Height <= height {
			strike = a
		}
	}
	return strike
}

// Len is how many glyphs the font holds, each of them in every strike
func (f *Font) Len() int { return len(f.runes) / 4 }

// Runes lists the glyphs the font holds in code point order
func (f *Font) Runes() []rune {
	runes := make([]rune, f.Len())
	for i := range runes {
		runes[i] = rune(binary.LittleEndian.Uint32(f.runes[4*i:]))
	}
	return runes
}

// index is r's place among the font's runes, by binary search
func (f *Font) index(r rune) (int, bool) {
	lo, hi := 0, f.Len()
	for lo < hi {
		mid := int(uint(lo+hi) / 2)
		if rune(binary.LittleEndian.Uint32(f.runes[4*mid:])) < r {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < f.Len() && rune(binary.LittleEndian.Uint32(f.runes[4*lo:])) == r
}

// Cell is the pixels a cell dp tall takes on a screen of density pixels to a dp,
// whole ones at the face's aspect and never under its smallest strike, and the
// strike that draws it: the largest no taller than the cell
func (f *Font) Cell(density, dp float64) (strike *Atlas, width, height int) {
	small, face := f.strikes[0], f.strikes[len(f.strikes)-1]
	// wide enough for the smallest strike's height too, should it be narrower than the face
	least := max(small.Width, (small.Height*face.Width+face.Height-1)/face.Height)
	width = max(least, int(math.Round(density*dp*float64(face.Width)/float64(face.Height))))
	height = width * face.Height / face.Width
	return f.Strike(height), width, height
}

// Has is whether the font holds a glyph for r, blank or not
func (a *Atlas) Has(r rune) bool {
	_, ok := a.font.index(r)
	return ok
}

// Aspect is the cell's width over its height: what a host drawing with this strike
// reports as its cell aspect
func (a *Atlas) Aspect() float64 { return float64(a.Width) / float64(a.Height) }

// glyph is the rows of the font's glyph i
func (a *Atlas) glyph(i int) []byte {
	n := a.Height * a.stride
	return a.rows[i*n:][:n]
}

// bits is row y of a glyph as a word, its leftmost column the highest bit
func (a *Atlas) bits(glyph []byte, y int) (bits uint32) {
	for i, b := range glyph[y*a.stride:][:a.stride] {
		bits |= uint32(b) << (24 - 8*i)
	}
	return bits
}

// Zoom is the range a host keeps a cell's height to as its player zooms, in dp:
// Least to Most in steps of Step
type Zoom struct{ Least, Most, Step float64 }

// Scale is a cell dp tall scaled by a zoom's factor: a step of the range, within it
// and never under f's smallest strike at the density; a range of no step keeps the
// cell
func (z Zoom) Scale(f *Font, density, dp, by float64) float64 {
	if z.Step <= 0 {
		return dp
	}
	least := max(z.Least, math.Ceil(float64(f.strikes[0].Height)/density/z.Step)*z.Step)
	return min(max(math.Round(dp*by/z.Step)*z.Step, least), z.Most)
}

// Texture is a strike as a cell drawer takes it: every glyph with a set bit as one
// slot of a mask, one byte a pixel, and the slot of each rune. A blank glyph has no
// slot and is painted as background.
type Texture struct {
	atlas *Atlas
	bytes []byte
	slots []int32 // by glyph, -1 for a blank one
}

// NewTexture encodes a as little-endian words and bytes: cell width, height and
// slot count, each slot's rows one below the last with 255 where the glyph is set,
// then each slot's rune as a word
func NewTexture(a *Atlas) *Texture {
	t := &Texture{atlas: a, slots: make([]int32, a.font.Len())}
	t.bytes = appendWords(t.bytes, uint32(a.Width), uint32(a.Height), 0)
	var runes []uint32
	for i := range t.slots {
		glyph := a.glyph(i)
		var set byte
		for _, b := range glyph {
			set |= b
		}
		if set == 0 {
			t.slots[i] = -1
			continue
		}
		t.slots[i] = int32(len(runes))
		runes = append(runes, binary.LittleEndian.Uint32(a.font.runes[4*i:]))
		for y := range a.Height {
			bits := a.bits(glyph, y)
			for shift := 31; shift > 31-a.Width; shift-- {
				t.bytes = append(t.bytes, byte(0xff*(bits>>shift&1)))
			}
		}
	}
	binary.LittleEndian.PutUint32(t.bytes[8:], uint32(len(runes)))
	t.bytes = appendWords(t.bytes, runes...)
	return t
}

// Atlas is the strike the texture encodes
func (t *Texture) Atlas() *Atlas { return t.atlas }

// Bytes is the encoded texture, as NewTexture lays it out
func (t *Texture) Bytes() []byte { return t.bytes }

// Slot is a rune's slot, -1 for a glyph painted as background only
func (t *Texture) Slot(r rune) int32 {
	if i, ok := t.atlas.font.index(r); ok {
		return t.slots[i]
	}
	return -1
}

// Frame is the terminal.FrameEncoder of the texture's drawer: a terminal.CellFrame
// whose ids are slots
func (t *Texture) Frame(dst []byte, cells []terminal.Cell, cols, rows int, changed []uint64) []byte {
	return terminal.CellFrame(dst, cells, cols, rows, changed, func(r rune) uint32 { return uint32(t.Slot(r)) })
}

func appendWords(dst []byte, words ...uint32) []byte {
	for _, w := range words {
		dst = binary.LittleEndian.AppendUint32(dst, w)
	}
	return dst
}
