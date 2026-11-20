package atlas

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/glyph"
)

// A frame never asks for a rune no cell draws as itself, so the face holds none,
// and it holds all of printable ASCII
func TestEveryRuneIsOneColumnAndPrintableASCIIIsThere(t *testing.T) {
	for _, r := range Hack().Runes() {
		if !terminal.OneColumn(r) {
			t.Errorf("%U is in the face and no cell draws it as itself", r)
		}
	}
	for r := ' '; r <= '~'; r++ {
		if !Hack().Strikes()[0].Has(r) {
			t.Errorf("%q is not in the face", r)
		}
	}
}

// Lines join from cell to cell at every size: a line sets pixels on the edges of
// the cell its shape reaches and on no other, the ones its crossing sets there,
// and the full block sets all
func TestLinesReachTheirCellEdgesAndTheFullBlockIsSolid(t *testing.T) {
	lines := []struct {
		r, cross rune
		edges    string // those of left, right, top, bottom the line reaches
	}{{'─', '┼', "lr"}, {'│', '┼', "tb"}, {'═', '╬', "lr"}, {'║', '╬', "tb"}}
	for _, a := range Hack().Strikes() {
		tex, w, h := glyph.NewTexture(a), a.Width, a.Height
		// bits is the pixels of r's glyph in a box of the cell, row by row
		bits := func(r rune, x0, y0, x1, y1 int) (bits uint64) {
			slot := int(tex.Slot(r))
			if slot < 0 {
				t.Fatalf("%dx%d: %c is blank", w, h, r)
			}
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					bits = bits<<1 | uint64(tex.Bytes()[12+(slot*h+y)*w+x]&1)
				}
			}
			return bits
		}
		edge := func(r, e rune) uint64 {
			x0, y0, x1, y1 := 0, 0, w, h
			switch e {
			case 'l':
				x1 = 1
			case 'r':
				x0 = w - 1
			case 't':
				y1 = 1
			case 'b':
				y0 = h - 1
			}
			return bits(r, x0, y0, x1, y1)
		}
		for _, l := range lines {
			for _, e := range "lrtb" {
				got, cross := edge(l.r, e), edge(l.cross, e)
				if want := strings.ContainsRune(l.edges, e); cross == 0 || (got != 0) != want || want && got != cross {
					t.Errorf("%dx%d: on edge %c, %c sets %b and %c sets %b", w, h, e, l.r, got, l.cross, cross)
				}
			}
		}
		for y := range h {
			if row := bits('█', 0, y, w, y+1); row != 1<<w-1 {
				t.Errorf("%dx%d: row %d of the full block is %b", w, h, y, row)
			}
		}
	}
}

// What a page is handed is a whole web font: woff2's signature, the length its
// header gives, and a size a face of some 1,500 glyphs comes to
func TestTheWebFontIsAWoff2(t *testing.T) {
	font := HackWeb()
	if len(font) < 12 || string(font[:4]) != "wOF2" {
		t.Fatalf("%d bytes that do not start wOF2", len(font))
	}
	if n := int(binary.BigEndian.Uint32(font[8:])); n != len(font) || n < 32<<10 || n > 256<<10 {
		t.Errorf("%d bytes, its header says %d: want them equal and between 32 and 256 KiB", len(font), n)
	}
}
