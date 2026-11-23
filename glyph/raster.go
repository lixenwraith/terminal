package glyph

import (
	"encoding/binary"
	"math/bits"

	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Raster draws a cell grid into an RGBA framebuffer with a strike. It is a
// terminal.Surface: Flush takes a whole frame and draws the cells that changed
// since the last one.
type Raster struct {
	atlas      *Atlas
	cols, rows int
	pix        []byte          // RGBA, cols*cw by rows*ch
	prev       []terminal.Cell // the frame drawn, so an unchanged cell is skipped
	diff       []uint64        // the cells a frame changes from prev
	cw, ch     int             // a cell's pixels: the strike's own until SetCell names others
	masks      [][]byte        // at another size, each glyph's coverage of the cell, nil until drawn
	fg, bg     color.RGB       // the colors mix holds, the last cell's that mixed any
	mix        [256]uint32     // a pixel of bg and fg at each level of coverage, indexed by a byte unchecked
}

// NewRaster draws with a at its own size, a cell a.Width by a.Height pixels
func NewRaster(a *Atlas) *Raster { return &Raster{atlas: a, cw: a.Width, ch: a.Height} }

// SetCell draws a cell width by height pixels from the next frame on, drawn whole.
// At the strike's own size its glyphs are drawn as they are. At another, each is
// scaled once to its coverage of the cell, a pixel taking the share of its area the
// glyph's set pixels lie over: a whole multiple is exact blocks, and a stem keeps
// one weight wherever it falls. The glyphs scaled for the size before are dropped.
func (r *Raster) SetCell(width, height int) {
	if width, height = max(width, 1), max(height, 1); width == r.cw && height == r.ch {
		return
	}
	r.cw, r.ch, r.masks = width, height, nil
	if width != r.atlas.Width || height != r.atlas.Height {
		r.masks = make([][]byte, r.atlas.font.Len())
	}
	r.cols, r.rows, r.pix, r.prev = 0, 0, nil, r.prev[:0]
}

// Cell is the pixels a cell is drawn at
func (r *Raster) Cell() (width, height int) { return r.cw, r.ch }

// ColorMode is truecolor: the raster paints RGB
func (r *Raster) ColorMode() terminal.ColorMode { return terminal.ColorModeTrueColor }

// Sync forgets the frame drawn, so the next one is drawn whole
func (r *Raster) Sync() { r.prev = r.prev[:0] }

// Pix is the framebuffer: RGBA rows of Bounds' width, top row first
func (r *Raster) Pix() []byte { return r.pix }

// Bounds is the framebuffer size in pixels
func (r *Raster) Bounds() (width, height int) { return r.cols * r.cw, r.rows * r.ch }

// Flush draws a frame of width by height cells, row-major; a frame short of its
// own size is dropped, as a terminal drops one
func (r *Raster) Flush(cells []terminal.Cell, width, height int) {
	n := width * height
	if width < 0 || height < 0 || len(cells) < n {
		return
	}
	if width != r.cols || height != r.rows {
		r.cols, r.rows = width, height
		r.pix = make([]byte, n*r.cw*r.ch*4)
		r.prev = r.prev[:0]
	}
	cells = cells[:n]
	paint := (*Raster).draw // picked here: a test inside draw moves its loop, which slows the strike's own size
	if r.masks != nil {
		paint = (*Raster).blend
	}
	if len(r.prev) != n {
		for i, c := range cells {
			paint(r, i%width, i/width, c)
		}
	} else {
		r.diff = terminal.ChangedCells(r.diff, cells, r.prev)
		for w, word := range r.diff {
			for ; word != 0; word &= word - 1 {
				i := w<<6 | bits.TrailingZeros64(word)
				paint(r, i%width, i/width, cells[i])
			}
		}
	}
	r.prev = append(r.prev[:0], cells...)
}

// Frame is the terminal.FrameEncoder of a drawer that shows the raster's pixels:
// little-endian words cols and rows, then Pix. It draws the cells that differ from
// its own last frame, every one for a nil changed.
func (r *Raster) Frame(dst []byte, cells []terminal.Cell, cols, rows int, changed []uint64) []byte {
	if changed == nil {
		r.Sync()
	}
	r.Flush(cells, cols, rows)
	return append(appendWords(dst, uint32(cols), uint32(rows)), r.pix...)
}

// draw paints one cell: its background, and its glyph in the foreground where
// the font has it
func (r *Raster) draw(cx, cy int, c terminal.Cell) {
	fg, bg := terminal.CellColors(c)
	var glyph []byte
	if i, ok := r.atlas.font.index(c.Rune); ok {
		glyph = r.atlas.glyph(i)
	}
	w, h := r.atlas.Width, r.atlas.Height
	stride := r.cols * w * 4
	for y := range h {
		var bits uint32
		if glyph != nil {
			bits = r.atlas.bits(glyph, y)
		}
		at := (cy*h+y)*stride + cx*w*4
		for x := range w {
			p := fg
			if bits&(1<<(31-x)) == 0 {
				p = bg
			}
			r.pix[at], r.pix[at+1], r.pix[at+2], r.pix[at+3] = p.R, p.G, p.B, 0xff
			at += 4
		}
	}
}

// shades is the levels a glyph's coverage of a pixel is kept in, from 0 for none
// of it to shades for all: a power of two, so a mix is a shift
const shades = 64

// blend paints one cell at a size other than the strike's: each pixel the cell's
// two colors mixed by the glyph's coverage of it
func (r *Raster) blend(cx, cy int, c terminal.Cell) {
	fg, bg := terminal.CellColors(c)
	var mask []byte
	if i, ok := r.atlas.font.index(c.Rune); ok {
		if mask = r.masks[i]; mask == nil {
			mask = r.cover(i)
			r.masks[i] = mask
		}
	}
	stride := r.cols * r.cw * 4
	at := cy*r.ch*stride + cx*r.cw*4
	if len(mask) == 0 { // a blank glyph, or a rune the font lacks
		top := r.pix[at:][:r.cw*4]
		back := uint32(bg.R) | uint32(bg.G)<<8 | uint32(bg.B)<<16 | 0xff<<24
		for x := 0; x < len(top); x += 4 {
			binary.LittleEndian.PutUint32(top[x:], back)
		}
		for y := 1; y < r.ch; y++ {
			copy(r.pix[at+y*stride:], top)
		}
		return
	}
	if fg != r.fg || bg != r.bg || r.mix[shades] == 0 { // other colors, or none mixed yet: a mix has alpha
		r.fg, r.bg = fg, bg
		// Each channel times shades, a half added so the shift rounds
		red, green, blue := shades*int32(bg.R)+shades/2, shades*int32(bg.G)+shades/2, shades*int32(bg.B)+shades/2
		for level := range r.mix[:shades+1] {
			r.mix[level] = uint32(red)/shades | uint32(green)/shades<<8 | uint32(blue)/shades<<16 | 0xff<<24
			red, green, blue = red+int32(fg.R)-int32(bg.R), green+int32(fg.G)-int32(bg.G), blue+int32(fg.B)-int32(bg.B)
		}
	}
	mix := &r.mix
	for ; len(mask) > 0; at += stride {
		row := r.pix[at:][:r.cw*4]
		for x := 0; x+4 <= len(row); x += 4 {
			binary.LittleEndian.PutUint32(row[x:x+4], mix[mask[x/4]])
		}
		mask = mask[r.cw:]
	}
}

// cover is glyph i's coverage of each of the cell's pixels, in shades, empty for a
// blank glyph. The strike's pixels are squares stretched over the cell; lengths
// are whole numbers in units a strike's pixel is r.cw or r.ch of and a cell's
// pixel a.Width or a.Height.
func (r *Raster) cover(i int) []byte {
	a := r.atlas
	glyph := a.glyph(i)
	var set byte
	for _, b := range glyph {
		set |= b
	}
	if set == 0 {
		return []byte{}
	}
	mask, row, area := make([]byte, r.cw*r.ch), make([]int, r.cw), a.Width*a.Height
	for y := range r.ch {
		clear(row)
		for sy := y * a.Height / r.ch; sy*r.ch < (y+1)*a.Height; sy++ {
			tall, bits := min((y+1)*a.Height, (sy+1)*r.ch)-max(y*a.Height, sy*r.ch), a.bits(glyph, sy)
			for x := range row {
				for sx := x * a.Width / r.cw; sx*r.cw < (x+1)*a.Width; sx++ {
					if bits&(1<<(31-sx)) != 0 {
						row[x] += tall * (min((x+1)*a.Width, (sx+1)*r.cw) - max(x*a.Width, sx*r.cw))
					}
				}
			}
		}
		for x, covered := range row {
			mask[y*r.cw+x] = byte((covered*shades + area/2) / area)
		}
	}
	return mask
}

// Probe is a frame of every glyph in the font, the color rules the raster applies
// taken in turn, with the raster's pixels for it: the oracle a drawer is checked
// against, same frame, same pixels
type Probe struct {
	tex        *Texture
	cols, rows int
	cells      []terminal.Cell
}

// NewProbe lays the probe out cols cells wide
func NewProbe(tex *Texture, cols int) *Probe {
	attrs := [...]terminal.Attr{0, terminal.AttrReverse, terminal.AttrDim,
		terminal.AttrFg256 | terminal.AttrBg256, terminal.AttrFgDefault | terminal.AttrBgDefault}
	runes := append(tex.atlas.font.Runes(), ' ', '一') // blank, and a rune the font lacks
	p := &Probe{tex: tex, cols: max(1, cols)}
	p.rows = (len(runes) + p.cols - 1) / p.cols
	p.cells = make([]terminal.Cell, p.cols*p.rows)
	for i, r := range runes {
		p.cells[i] = terminal.Cell{
			Rune:  r,
			Fg:    color.RGB{R: uint8(i * 37), G: uint8(i * 91), B: uint8(255 - i)},
			Bg:    color.RGB{R: uint8(i * 13), G: 40, B: uint8(i * 7)},
			Attrs: attrs[i%len(attrs)],
		}
	}
	return p
}

// Cells is the probe's frame and its size, to Flush to a Surface
func (p *Probe) Cells() (cells []terminal.Cell, cols, rows int) { return p.cells, p.cols, p.rows }

// Frame is the probe as a whole cell frame, as the texture's Frame encodes one
func (p *Probe) Frame() []byte {
	return p.tex.Frame(nil, p.cells, p.cols, p.rows, nil)
}

// Pixels is the frame as Raster draws it, RGBA rows top first
func (p *Probe) Pixels() []byte {
	r := NewRaster(p.tex.atlas)
	r.Flush(p.cells, p.cols, p.rows)
	return r.Pix()
}
