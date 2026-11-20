package glyph

import (
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
	pix        []byte          // RGBA, cols*Width by rows*Height
	prev       []terminal.Cell // the frame drawn, so an unchanged cell is skipped
	diff       []uint64        // the cells a frame changes from prev
}

func NewRaster(a *Atlas) *Raster { return &Raster{atlas: a} }

// ColorMode is truecolor: the raster paints RGB
func (r *Raster) ColorMode() terminal.ColorMode { return terminal.ColorModeTrueColor }

// Sync forgets the frame drawn, so the next one is drawn whole
func (r *Raster) Sync() { r.prev = r.prev[:0] }

// Pix is the framebuffer: RGBA rows of Bounds' width, top row first
func (r *Raster) Pix() []byte { return r.pix }

// Bounds is the framebuffer size in pixels
func (r *Raster) Bounds() (width, height int) {
	return r.cols * r.atlas.Width, r.rows * r.atlas.Height
}

// Flush draws a frame of width by height cells, row-major; a frame short of its
// own size is dropped, as a terminal drops one
func (r *Raster) Flush(cells []terminal.Cell, width, height int) {
	n := width * height
	if width < 0 || height < 0 || len(cells) < n {
		return
	}
	if width != r.cols || height != r.rows {
		r.cols, r.rows = width, height
		r.pix = make([]byte, n*r.atlas.Width*r.atlas.Height*4)
		r.prev = r.prev[:0]
	}
	cells = cells[:n]
	if len(r.prev) != n {
		for i, c := range cells {
			r.draw(i%width, i/width, c)
		}
	} else {
		r.diff = terminal.ChangedCells(r.diff, cells, r.prev)
		for w, word := range r.diff {
			for ; word != 0; word &= word - 1 {
				i := w<<6 | bits.TrailingZeros64(word)
				r.draw(i%width, i/width, cells[i])
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

// Probe is a frame of every glyph in the font under each color rule the raster
// applies, with the raster's pixels for it: the oracle a drawer is checked against,
// same frame, same pixels
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
