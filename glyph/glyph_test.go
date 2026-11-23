package glyph

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// strike is a cell size and its glyphs, each as rows of . and #
type strike struct {
	w, h   int
	glyphs map[rune][]string
}

// pack writes a font file of strikes, smallest first, that hold the same runes
func pack(name string, strikes ...strike) []byte {
	le := binary.LittleEndian
	runes := slices.Sorted(maps.Keys(strikes[0].glyphs))
	b := le.AppendUint16(le.AppendUint16([]byte(magic), uint16(len(strikes))), uint16(len(name)))
	b = append(le.AppendUint32(b, uint32(len(runes))), name...)
	b = append(b, 0, 0, 0)[:(len(b)+3)&^3]
	for _, r := range runes {
		b = le.AppendUint32(b, uint32(r))
	}
	for _, s := range strikes {
		b = le.AppendUint16(le.AppendUint16(b, uint16(s.w)), uint16(s.h))
		for _, r := range runes {
			for _, row := range s.glyphs[r] {
				for at := 0; at < s.w; at += 8 {
					var bits byte
					for x, c := range row[at:min(at+8, s.w)] {
						bits |= byte(c&1) << (7 - x) // '#' is odd and '.' even
					}
					b = append(b, bits)
				}
			}
		}
	}
	return b
}

// unpack reads a font back as pack takes it
func unpack(f *Font) []strike {
	strikes := make([]strike, len(f.Strikes()))
	for i, a := range f.Strikes() {
		strikes[i] = strike{a.Width, a.Height, map[rune][]string{}}
		for g, r := range f.Runes() {
			rows := make([]string, a.Height)
			for y := range rows {
				row := fmt.Sprintf("%032b", a.bits(a.glyph(g), y))[:a.Width]
				rows[y] = strings.NewReplacer("0", ".", "1", "#").Replace(row)
			}
			strikes[i].glyphs[r] = rows
		}
	}
	return strikes
}

// twoFile is a face of two strikes, the wider one's rows two bytes each; its dot is
// blank in the smaller
var twoFile = pack("two",
	strike{4, 2, map[rune][]string{
		'x': {"#..#", ".##."},
		'"': {"##..", "...."},
		'.': {"....", "...."},
	}},
	strike{9, 3, map[rune][]string{
		'x': {"##......#", "....#....", "#.......#"},
		'"': {"###...#..", "###......", "........."},
		'.': {".........", ".........", "...#....."},
	}})

func parse(t testing.TB, file []byte) *Font {
	t.Helper()
	f, err := Parse(file)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestARasterDrawsTheAtlasInTheCellsColors: a glyph's set bits take the cell's
// foreground and the rest its background, a row wider than a byte running on into
// the next; reverse swaps them and a palette index resolves to xterm's color; a
// cell unchanged since the last frame is not drawn again, until a frame of another
// shape, a Sync or a raster frame of every cell draws them all.
func TestARasterDrawsTheAtlasInTheCellsColors(t *testing.T) {
	f := parse(t, twoFile)
	a := f.Strike(2)
	if f.Len() != 3 || a.Width != 4 || a.Height != 2 || a.Aspect() != 2 || !a.Has('.') || a.Has('y') {
		t.Fatalf("strike %dx%d of a font of %d glyphs", a.Width, a.Height, f.Len())
	}
	r := NewRaster(a)
	fg, bg := color.RGB{R: 255}, color.RGB{B: 255}
	frame := []terminal.Cell{
		{Rune: 'x', Fg: fg, Bg: bg},
		{Rune: 'x', Fg: fg, Bg: bg, Attrs: terminal.AttrReverse},
		{Rune: ' ', Fg: color.RGB{R: 196}, Attrs: terminal.AttrFg256 | terminal.AttrBg256}, // index 196 is pure red; bg index 0 is black
	}
	r.Flush(frame, 3, 1)
	if w, h := r.Bounds(); w != 12 || h != 2 {
		t.Fatalf("bounds %dx%d, want 12x2", w, h)
	}
	at := func(x, y int) color.RGB {
		w, _ := r.Bounds()
		i := (y*w + x) * 4
		return color.RGB{R: r.pix[i], G: r.pix[i+1], B: r.pix[i+2]}
	}
	if at(0, 0) != fg || at(1, 0) != bg || at(1, 1) != fg || at(0, 1) != bg {
		t.Fatalf("cell 0 drew %v %v / %v %v", at(0, 0), at(1, 0), at(0, 1), at(1, 1))
	}
	if at(4, 0) != bg || at(5, 0) != fg {
		t.Fatalf("reversed cell drew %v %v", at(4, 0), at(5, 0))
	}
	if got := at(8, 0); got != (color.RGB{}) {
		t.Fatalf("palette background drew %v, want black", got)
	}

	// The second frame changes one cell; the raster must keep the others as drawn
	frame[2] = terminal.Cell{Rune: '"', Fg: color.RGB{R: 196}, Attrs: terminal.AttrFg256}
	mark := color.RGB{R: 1} // on cell 0, where only drawing the cell again clears it
	r.pix[0] = mark.R
	r.Flush(frame, 3, 1)
	if at(8, 0) != (color.RGB{R: 0xff}) || at(0, 0) != mark {
		t.Fatalf("second frame drew %v and kept %v", at(8, 0), at(0, 0))
	}
	r.Flush(frame, 1, 3)
	if w, h := r.Bounds(); w != 4 || h != 6 || at(0, 0) != fg || at(0, 4) != (color.RGB{R: 0xff}) {
		t.Fatalf("the frame stood on end is %dx%d and drew %v and %v", w, h, at(0, 0), at(0, 4))
	}
	r.pix[0] = mark.R
	r.Sync()
	if r.Flush(frame, 1, 3); at(0, 0) != fg {
		t.Fatal("the frame after a Sync kept a cell as drawn")
	}
	r.pix[0] = mark.R
	if out := r.Frame([]byte{9}, frame, 1, 3, []uint64{0}); at(0, 0) != mark || len(out) != 9+len(r.pix) {
		t.Fatalf("a raster frame of no change is %d bytes and drew %v", len(out), at(0, 0))
	}
	out := r.Frame([]byte{9}, frame, 1, 3, nil)
	if at(0, 0) != fg || !bytes.Equal(out[:9], []byte{9, 1, 0, 0, 0, 3, 0, 0, 0}) || !bytes.Equal(out[9:], r.pix) {
		t.Fatalf("a raster frame of every cell starts % x and drew %v", out[:9], at(0, 0))
	}

	r = NewRaster(f.Strike(3))
	r.Flush(frame, 1, 1)
	if at(1, 0) != fg || at(7, 0) != bg || at(8, 0) != fg || at(4, 1) != fg || at(8, 1) != bg {
		t.Fatalf("the 9 wide x drew %v %v %v / %v %v", at(1, 0), at(7, 0), at(8, 0), at(4, 1), at(8, 1))
	}
}

// scaledFrame is a frame of the fixture's every glyph, one reversed, a blank and a
// rune the font lacks, in colors no two channels of which blend alike
func scaledFrame() ([]terminal.Cell, int, int) {
	fg, bg := color.RGB{R: 200, G: 100}, color.RGB{G: 100, B: 200}
	return []terminal.Cell{
		{Rune: 'x', Fg: fg, Bg: bg}, {Rune: '"', Fg: fg, Bg: bg, Attrs: terminal.AttrReverse},
		{Rune: '.', Fg: fg, Bg: bg}, {Rune: 'y', Fg: fg, Bg: bg},
		{Rune: '"', Fg: fg, Bg: bg}, {Rune: 'x', Fg: color.RGB{R: 196}, Attrs: terminal.AttrFg256 | terminal.AttrBg256},
	}, 3, 2
}

// TestAGlyphScaledToItsOwnSizeIsDrawnAsItIs: a glyph's coverage of a cell of the
// strike's own size is its pixels, set or not, so the raster that scales draws
// there, byte for byte, what the raster that does not draws
func TestAGlyphScaledToItsOwnSizeIsDrawnAsItIs(t *testing.T) {
	cells, cols, rows := scaledFrame()
	for _, a := range parse(t, twoFile).Strikes() {
		own, scaled := NewRaster(a), NewRaster(a)
		scaled.masks = make([][]byte, a.font.Len()) // as SetCell leaves a raster of another size
		own.Flush(cells, cols, rows)
		scaled.Flush(cells, cols, rows)
		if !bytes.Equal(own.pix, scaled.pix) {
			t.Errorf("the %dx%d strike scaled to its own size differs from itself", a.Width, a.Height)
		}
	}
}

// TestAWholeMultipleOfTheStrikeIsItsPixelsAsBlocks: a cell a whole number of the
// strike's each way is the strike's frame with each pixel a block of that many, as
// nearest scaling gives it, whatever size the raster drew at before
func TestAWholeMultipleOfTheStrikeIsItsPixelsAsBlocks(t *testing.T) {
	cells, cols, rows := scaledFrame()
	for _, a := range parse(t, twoFile).Strikes() {
		own, scaled := NewRaster(a), NewRaster(a)
		own.Flush(cells, cols, rows)
		scaled.SetCell(a.Width+1, a.Height+2) // glyphs scaled for a size that is left
		scaled.Flush(cells, cols, rows)
		const kx, ky = 2, 3
		scaled.SetCell(kx*a.Width, ky*a.Height)
		if w, h := scaled.Bounds(); w != 0 || h != 0 {
			t.Fatalf("a raster told another cell still holds a frame of %dx%d pixels", w, h)
		}
		scaled.Flush(cells, cols, rows)
		w, h := own.Bounds()
		if sw, sh := scaled.Bounds(); sw != kx*w || sh != ky*h {
			t.Fatalf("the frame of %dx%d cells is %dx%d pixels, want %dx%d", kx*a.Width, ky*a.Height, sw, sh, kx*w, ky*h)
		}
		for y := range ky * h {
			for x := range kx * w {
				if got, want := scaled.pix[(y*kx*w+x)*4:][:4], own.pix[(y/ky*w+x/kx)*4:][:4]; !bytes.Equal(got, want) {
					t.Fatalf("the %dx%d strike: pixel %d,%d is % x, want the strike's %d,%d, % x", a.Width, a.Height, x, y, got, x/kx, y/ky, want)
				}
			}
		}
	}
}

// TestAScaledPixelTakesTheShareOfItTheGlyphCovers: at a cell half as large again
// a pixel lies over one of the strike's or half over each of two, each way, and
// mixes the cell's colors by the share of it the glyph's set pixels cover
func TestAScaledPixelTakesTheShareOfItTheGlyphCovers(t *testing.T) {
	r := NewRaster(parse(t, twoFile).Strike(2)) // x is #..# over .##.
	r.SetCell(6, 3)
	if w, h := r.Cell(); w != 6 || h != 3 {
		t.Fatalf("a raster told a 6x3 cell draws one %dx%d", w, h)
	}
	white := color.RGB{R: 255, G: 255, B: 255}
	r.Flush([]terminal.Cell{{Rune: 'x', Fg: white}}, 1, 1)
	var got [18]byte
	for i := range got {
		got[i] = r.pix[4*i]
	}
	if want := [18]byte{255, 128, 0, 0, 128, 255, 128, 128, 128, 128, 128, 128, 0, 128, 255, 255, 128, 0}; got != want {
		t.Errorf("x covers the 6x3 cell by %v, want %v", got, want)
	}
	r.Flush([]terminal.Cell{{Rune: 'x', Fg: color.RGB{R: 200, G: 100}, Bg: color.RGB{G: 100, B: 200}}}, 1, 1)
	if got, want := r.pix[4:8], []byte{100, 100, 100, 255}; !bytes.Equal(got, want) {
		t.Errorf("a pixel half covered is % x, want the colors half and half, % x", got, want)
	}
}

// TestAScaledFrameDrawsTheCellsThatChangedAlone: after one cell changed, a raster
// that scales paints that cell's pixels as a whole frame would and no other
func TestAScaledFrameDrawsTheCellsThatChangedAlone(t *testing.T) {
	cells, cols, rows := scaledFrame()
	a := parse(t, twoFile).Strike(2)
	r, whole := NewRaster(a), NewRaster(a)
	r.SetCell(6, 3)
	whole.SetCell(6, 3)
	r.Flush(cells, cols, rows)
	for i := range r.pix {
		r.pix[i] ^= 0x55 // every pixel marked: only a cell drawn again loses the mark
	}
	const changed = 4 // the cell at column 1 of row 1
	cells[changed].Rune = 'x'
	r.Flush(cells, cols, rows)
	whole.Flush(cells, cols, rows)
	for i, b := range r.pix {
		x, y := i/4%(cols*6), i/4/(cols*6)
		want := whole.pix[i] ^ 0x55
		if x/6 == changed%cols && y/3 == changed/cols {
			want = whole.pix[i]
		}
		if b != want {
			t.Fatalf("pixel %d,%d holds %#x in byte %d, want %#x", x, y, b, i%4, want)
		}
	}
}

// BenchmarkRasterFlush draws a 120x40 frame of a 12x24 strike of 300 glyphs, of
// which 15 % of the cells changed, at the strike's own size and scaled to a cell
// half as large again
func BenchmarkRasterFlush(b *testing.B) {
	glyphs := map[rune][]string{}
	for r := range rune(300) { // a box each: strokes, as a face's glyphs are, and no two alike
		left, right, top, foot := int(r%5), 6+int(r%6), 2+int(r%7), 12+int(r%11)
		rows := slices.Repeat([]string{"............"}, 24)
		for y := top; y <= foot; y++ {
			rows[y] = rows[y][:left] + "#" + rows[y][left+1:right] + "#" + rows[y][right+1:]
		}
		rows[top] = rows[top][:left] + strings.Repeat("#", right-left) + rows[top][right:]
		rows[foot] = rows[top]
		glyphs[' '+r] = rows
	}
	a := parse(b, pack("bench", strike{12, 24, glyphs})).Strike(24)
	const w, h = 120, 40
	frames := [2][]terminal.Cell{make([]terminal.Cell, w*h), make([]terminal.Cell, w*h)}
	for i := range frames[0] {
		frames[0][i] = terminal.Cell{Rune: 'x', Fg: color.RGB{R: uint8(i)}, Bg: color.RGB{B: 20}}
		frames[1][i] = frames[0][i]
		if i%7 == 0 {
			frames[1][i].Rune = '"'
		}
	}
	for _, cell := range [][2]int{{12, 24}, {18, 36}} {
		b.Run(fmt.Sprintf("cell=%dx%d", cell[0], cell[1]), func(b *testing.B) {
			r := NewRaster(a)
			r.SetCell(cell[0], cell[1])
			r.Flush(frames[0], w, h)
			r.Flush(frames[1], w, h) // both frames' glyphs are scaled before the clock runs
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				r.Flush(frames[i%2], w, h)
			}
		})
	}
}

// TestACellFramePaintedFromTheTextureIsTheRastersPixels: painting each record's
// background and then its glyph from the texture in its foreground, as a cell
// drawer does, gives the raster's pixels for the probe and for a live frame; the
// texture names each slot's rune after the masks, and a strike has slots of its
// own, for the glyphs it sets a bit of.
func TestACellFramePaintedFromTheTextureIsTheRastersPixels(t *testing.T) {
	f := parse(t, twoFile)
	for i, want := range []struct{ slots, dot, x int32 }{{2, -1, 1}, {3, 1, 2}} {
		a := f.Strikes()[i]
		tex := NewTexture(a)
		mask := tex.Bytes()
		n := int32(binary.LittleEndian.Uint32(mask[8:]))
		if n != want.slots || len(mask) != 12+int(n)*(a.Width*a.Height+4) || mask[12] != 0xff {
			t.Fatalf("texture of %d bytes holds %d slots, want %d, the first set pixel %d", len(mask), n, want.slots, mask[12])
		}
		if r := rune(binary.LittleEndian.Uint32(mask[len(mask)-4*int(want.slots):])); r != '"' || tex.Slot(r) != 0 ||
			tex.Slot('.') != want.dot || tex.Slot('x') != want.x || tex.Slot('y') != -1 {
			t.Fatalf("slot 0 is %q; the slots of the dot, x and y are %d, %d and %d", r, tex.Slot('.'), tex.Slot('x'), tex.Slot('y'))
		}
		p := NewProbe(tex, 3)
		if got, want := paint(t, mask, p.Frame()), p.Pixels(); !bytes.Equal(got, want) {
			t.Fatal("the probe painted from the texture differs from the raster")
		}
		cell, raster := terminal.NewFrames(tex.Frame), terminal.NewFrames(NewRaster(a).Frame)
		for _, f := range []*terminal.Frames{cell, raster} {
			f.Flush(p.Cells())
		}
		frame := raster.Take()
		if got := paint(t, mask, cell.Take()); !bytes.Equal(got, frame[8:]) {
			t.Fatal("a live cell frame differs from the raster drawer's")
		}
	}
}

// paint is a cell drawer in Go: RGBA from a whole cell frame and the texture
func paint(t *testing.T, mask, frame []byte) []byte {
	t.Helper()
	word := func(b []byte, i int) uint32 { return binary.LittleEndian.Uint32(b[4*i:]) }
	w, h := int(word(mask, 0)), int(word(mask, 1))
	cols, rows, n := int(word(frame, 0)), int(word(frame, 1)), int(word(frame, 2))
	if n != cols*rows {
		t.Fatalf("frame of %d records for %dx%d cells", n, cols, rows)
	}
	pix := make([]byte, cols*w*rows*h*4)
	for r := range n {
		idx, slot, fg, bg := int(word(frame, 3+4*r)), int32(word(frame, 4+4*r)), word(frame, 5+4*r), word(frame, 6+4*r)
		for y := range h {
			for x := range w {
				c := bg
				if slot >= 0 && mask[12+(int(slot)*h+y)*w+x] != 0 {
					c = fg
				}
				at := (((idx/cols)*h+y)*cols*w + (idx%cols)*w + x) * 4
				pix[at], pix[at+1], pix[at+2], pix[at+3] = byte(c>>16), byte(c>>8), byte(c), byte(c>>24)
			}
		}
	}
	return pix
}

// broken is twoFile off its layout in each way Parse refuses
func broken() map[string][]byte {
	edit := func(file []byte, at int, b byte) []byte {
		file = bytes.Clone(file)
		file[at] = b
		return file
	}
	return map[string][]byte{
		"another magic":                          edit(twoFile, 3, '2'),
		"no strike":                              edit(twoFile[:28], 4, 0),
		"its runes cut short":                    twoFile[:20],
		"a strike fewer than it counts":          edit(twoFile, 4, 3),
		"a byte short":                           twoFile[:len(twoFile)-1],
		"a byte over":                            append(bytes.Clone(twoFile), 0),
		"a name that is not UTF-8":               edit(twoFile, 12, 0xff),
		"a set byte after the name":              edit(twoFile, 15, 1),
		"a rune twice":                           edit(twoFile, 20, '"'),
		"a rune past the last code point":        edit(twoFile, 26, 0x11),
		"a strike no pixel wide":                 pack("", strike{0, 1, nil}),
		"a strike 33 pixels wide":                pack("", strike{33, 1, nil}),
		"a strike no pixel high":                 pack("", strike{1, 0, nil}),
		"a strike no taller than the one before": pack("", strike{4, 2, nil}, strike{5, 2, nil}),
		"a bit set past a row's width":           edit(twoFile, 32, 0xc1),
		"a bit set past a two byte row's width":  edit(twoFile, 43, 0x40),
	}
}

// TestAFileOffTheLayoutIsRefused: Parse takes a font file and nothing that
// differs from one in its magic, its sizes, its name, the order of its runes, a
// strike's size or order, or a bit past a row's width.
func TestAFileOffTheLayoutIsRefused(t *testing.T) {
	parse(t, twoFile)
	for what, file := range broken() {
		if _, err := Parse(file); err == nil {
			t.Errorf("a file with %s was taken", what)
		}
	}
}

// FuzzParse: Parse returns whatever the bytes, and a file it takes is one pack
// writes, read back without leaving it
func FuzzParse(f *testing.F) {
	f.Add(twoFile)
	for _, file := range broken() {
		f.Add(file)
	}
	f.Fuzz(func(t *testing.T, file []byte) {
		font, err := Parse(file)
		if err != nil {
			return
		}
		if again := pack(font.Name, unpack(font)...); !bytes.Equal(again, file) {
			t.Fatalf("a file was taken that packs again as another:\n%x\n%x", file, again)
		}
	})
}

// TestACellIsDrawnByTheLargestStrikeItHolds: a strike is picked by height, the
// smallest under them all; a cell is whole pixels at the face's aspect and holds
// the smallest strike, one off that aspect too; a zoom stops at that strike's
// height, fewer dp on a denser screen.
func TestACellIsDrawnByTheLargestStrikeItHolds(t *testing.T) {
	sizes := func(wh ...int) *Font {
		var strikes []strike
		for i := 0; i < len(wh); i += 2 {
			strikes = append(strikes, strike{w: wh[i], h: wh[i+1]})
		}
		return parse(t, pack("", strikes...))
	}
	f := sizes(6, 12, 8, 16, 12, 24)
	for height, want := range map[int]int{0: 12, 12: 12, 15: 12, 16: 16, 23: 16, 24: 24, 99: 24} {
		if got := f.Strike(height).Height; got != want {
			t.Errorf("%d pixels are drawn by the strike %d high, want %d", height, got, want)
		}
	}
	for _, c := range []struct {
		f           *Font
		density, dp float64
		strike      int
		w, h        int
	}{
		{f, 1, 24, 24, 12, 24},
		{f, 1, 20, 16, 10, 20},
		{f, 1, 13, 12, 7, 14},
		{f, 1, 4, 12, 6, 12},
		{f, 2.5, 7, 16, 9, 18},
		{f, 3, 12, 24, 18, 36},
		{sizes(12, 24), 1, 4, 24, 12, 24},
		{sizes(5, 12, 12, 20), 1, 4, 12, 8, 13},
	} {
		if a, w, h := c.f.Cell(c.density, c.dp); a.Height != c.strike || w != c.w || h != c.h {
			t.Errorf("%g dp at density %g: a %dx%d cell of the strike %d high, want %dx%d of %d",
				c.dp, c.density, w, h, a.Height, c.w, c.h, c.strike)
		}
	}
	z := Zoom{Least: 4, Most: 40, Step: 0.5}
	for _, c := range []struct{ density, dp, by, want float64 }{
		{1, 24, 1.26, 30},
		{1, 24, 0.5, 12},
		{1, 12, 0.5, 12},
		{2, 12, 0.25, 6},
		{4, 12, 0.1, 4},
		{1, 24, 10, 40},
	} {
		if got := z.Scale(f, c.density, c.dp, c.by); got != c.want {
			t.Errorf("%g dp scaled by %g at density %g is %g, want %g", c.dp, c.by, c.density, got, c.want)
		}
	}
	if got := (Zoom{}).Scale(f, 1, 17, 3); got != 17 {
		t.Errorf("a range of no step scaled 17 dp to %g", got)
	}
}
