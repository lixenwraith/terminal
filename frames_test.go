package terminal

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/lixenwraith/color"
)

// TestTheDrawerTakesTheNewestFrameAsChangesFromTheOneItHolds: a frame not taken is
// replaced by the next, the take carries every cell that differs from what the
// drawer holds and nothing when none does, a resize or a Sync makes it whole, a new
// encoder hands over whole the frame it holds, and Blocked counts from the oldest
// frame waiting.
func TestTheDrawerTakesTheNewestFrameAsChangesFromTheOneItHolds(t *testing.T) {
	f := NewFrames(nil)
	if f.Take() != nil || f.Blocked() != 0 {
		t.Fatal("frames nothing was flushed to hold one")
	}
	cells := make([]Cell, 4*2)
	for i := range cells {
		cells[i] = Cell{Rune: 'x', Fg: color.RGB{R: 200}}
	}
	f.Flush(cells, 4, 2)
	if n := records(t, f.Take()); n != 8 {
		t.Fatalf("the first frame carried %d cells, want all 8", n)
	}

	cells[1].Rune = '"'
	f.Flush(cells, 4, 2)
	time.Sleep(5 * time.Millisecond)
	cells[1].Rune, cells[6].Rune = 'x', ' ' // the drawer never saw the quote
	f.Flush(cells, 4, 2)
	if f.Blocked() < 5*time.Millisecond {
		t.Fatalf("blocked %v counts from the newest frame, not the oldest waiting", f.Blocked())
	}
	if n := records(t, f.Take()); n != 1 {
		t.Fatalf("the take carried %d cells, want the one that differs from the drawer's", n)
	}
	if f.Take() != nil || f.Blocked() != 0 {
		t.Fatal("a frame was taken twice")
	}

	f.Flush(cells, 4, 2)
	cells[0].Rune = 'y'      // the caller's own again, no part of the frame flushed
	f.Flush(cells[:7], 4, 2) // short of its size: dropped
	if fr := f.Take(); fr != nil {
		t.Fatalf("a frame the drawer already holds was handed over again: %d cells", records(t, fr))
	}
	f.Flush(cells, 4, 2)
	f.Sync()
	if n := records(t, f.Take()); n != 8 {
		t.Fatalf("after Sync the take carried %d cells, want all 8", n)
	}
	f.SetEncoder(func(dst []byte, cells []Cell, cols, rows int, changed []uint64) []byte {
		return CellFrame(dst, cells, cols, rows, changed, func(rune) uint32 { return 7 })
	})
	if fr := f.Take(); records(t, fr) != 8 || binary.LittleEndian.Uint32(fr[16:]) != 7 {
		t.Fatal("after a new encoder the take is not the drawer's frame whole, as that encoder writes it")
	}
	f.Flush(cells, 2, 4)
	if n := records(t, f.Take()); n != 8 {
		t.Fatalf("after a resize the take carried %d cells, want all 8", n)
	}
}

func records(t *testing.T, frame []byte) int {
	t.Helper()
	if len(frame) < 12 {
		t.Fatalf("frame of %d bytes has no header", len(frame))
	}
	n := int(binary.LittleEndian.Uint32(frame[8:]))
	if len(frame) != 12+16*n {
		t.Fatalf("frame of %d bytes holds %d records", len(frame), n)
	}
	return n
}

// TestARuneFrameCarriesEachCellAsADrawerPaintsIt: a record is the cell's index, the
// rune Flush would draw for it, and its colors resolved: reverse swaps them, dim
// then halves the foreground, a palette index is xterm's color and the terminal's
// own are light grey on black; a frame of changes carries the marked cells alone
func TestARuneFrameCarriesEachCellAsADrawerPaintsIt(t *testing.T) {
	red, blue := color.RGB{R: 0xff}, color.RGB{B: 0xff}
	cells := []Cell{
		{Rune: 'x', Fg: red, Bg: blue},
		{Rune: '一', Fg: red, Bg: color.RGB{G: 0x64}, Attrs: AttrReverse | AttrDim},
		{Fg: color.RGB{R: 0xc8, G: 0x64, B: 0x0a}, Bg: blue, Attrs: AttrDim | AttrBold},
		{Rune: '─', Fg: color.RGB{R: 196}, Bg: blue, Attrs: AttrFg256 | AttrBgDefault},
		{Rune: 'y', Fg: red, Bg: color.RGB{R: 21}, Attrs: AttrFgDefault | AttrBg256},
	}
	want := []uint32{5, 1, 5,
		0, 'x', 0xffff0000, 0xff0000ff,
		1, 0xfffd, 0xff003200, 0xffff0000,
		2, ' ', 0xff643205, 0xff0000ff,
		3, '─', 0xffff0000, 0xff000000,
		4, 'y', 0xffc0c0c0, 0xff0000ff,
	}
	words := func(frame []byte) []uint32 {
		w := make([]uint32, len(frame)/4)
		for i := range w {
			w[i] = binary.LittleEndian.Uint32(frame[4*i:])
		}
		return w
	}
	if got := words(RuneFrame(nil, cells, 5, 1, nil)); !slices.Equal(got, want) {
		t.Fatalf("a whole frame is %#x, want %#x", got, want)
	}
	wide := slices.Concat(cells[:2], make([]Cell, 63), cells[4:]) // y is cell 65, in the second word of marks
	want = slices.Concat([]uint32{66, 1, 2}, want[7:11], []uint32{65}, want[20:])
	if got := words(RuneFrame(nil, wide, 66, 1, []uint64{1 << 1, 1 << 1})); !slices.Equal(got, want) {
		t.Fatalf("a frame of cells 1 and 65 is %#x, want %#x", got, want)
	}
}
