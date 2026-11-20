package android

import (
	"bytes"
	"testing"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/atlas"
	"github.com/lixenwraith/terminal/glyph"
)

// pinch is the range the tests' views zoom in
var pinch = glyph.Zoom{Least: 4, Most: 40, Step: 0.5}

func testView(raster bool, density float32) *View {
	return NewView(Config{Font: atlas.Hack(), Raster: raster, Density: density, Zoom: pinch, BarRows: 2})
}

// TestATapPressesAndReleasesTheCellUnderIt: a 420 dpi phone's cell is 16x32
// pixels, and the grid they fit is stretched over the view, so a pixel maps to the
// cell drawn there in proportion, one past the edge to the edge cell; nothing is
// reported while the program takes no pointer.
func TestATapPressesAndReleasesTheCellUnderIt(t *testing.T) {
	v := testView(false, 2.625)
	v.Resize(10*16+15, 4*32+31) // 10x4 cells, stretched by most of a cell each way
	if cols, rows := v.Size(); cols != 10 || rows != 4 {
		t.Fatalf("the grid is %dx%d, want 10x4", cols, rows)
	}
	if ev, ok := v.touch(30, 30, true); ok {
		t.Fatalf("a touch with the pointer off was reported: %+v", ev)
	}
	v.SetPointerMode(true, false)
	for _, c := range []struct {
		x, y, cx, cy int
		down         bool
	}{
		{0, 0, 0, 0, true}, {17, 31, 0, 0, true}, {18, 32, 1, 0, true}, {174, 158, 9, 3, true},
		{175, 159, 9, 3, false}, {-4, 400, 0, 3, false},
	} {
		want := terminal.Event{Type: terminal.EventMouse, MouseBtn: terminal.MouseBtnLeft,
			MouseAction: terminal.MouseActionRelease, MouseX: c.cx, MouseY: c.cy, Touch: true}
		if c.down {
			want.MouseAction = terminal.MouseActionPress
		}
		if ev, ok := v.touch(c.x, c.y, c.down); !ok || ev != want {
			t.Errorf("touch at %d,%d is %+v %v, want %+v", c.x, c.y, ev, ok, want)
		}
	}
}

// TestAFingerDragsOncePerCell: a pressed finger moving reports a left drag each
// time it enters another cell and nothing within one, past the view's edge on its
// edge cell; a finger not pressed drags nothing.
func TestAFingerDragsOncePerCell(t *testing.T) {
	const w, h = 12, 24
	v := testView(false, 2)
	v.Resize(10*w, 4*h)
	v.SetPointerMode(true, false)
	drag := func(x, y int) terminal.Event {
		return terminal.Event{Type: terminal.EventMouse, MouseBtn: terminal.MouseBtnLeft,
			MouseAction: terminal.MouseActionDrag, MouseX: x, MouseY: y, Touch: true}
	}
	if ev, ok := v.drag(w*2, h); ok {
		t.Fatalf("a finger never pressed dragged: %+v", ev)
	}
	v.touch(w*3+1, h*2+1, true)
	for _, c := range []struct {
		x, y int
		want terminal.Event
		ok   bool
	}{
		{w*3 + 9, h*2 + 20, terminal.Event{}, false},
		{w*4 + 1, h*2 + 1, drag(4, 2), true},
		{w*4 + 11, h * 2, terminal.Event{}, false},
		{w * 4, h*3 + 2, drag(4, 3), true},
		{w*10 + 4, h * 3, drag(9, 3), true},
		{w*10 + 9, h * 3, terminal.Event{}, false},
	} {
		if ev, ok := v.drag(c.x, c.y); ok != c.ok || ev != c.want {
			t.Errorf("drag to %d,%d is %+v %v, want %+v %v", c.x, c.y, ev, ok, c.want, c.ok)
		}
	}
	v.touch(w*9, h*3, false)
	if ev, ok := v.drag(w, h); ok {
		t.Fatalf("a released finger dragged: %+v", ev)
	}
}

// TestKeysArriveAsATerminalReportsThem: a named key, Ctrl with a letter as its
// control byte, Shift-Tab as back-tab and a typed character are the events a
// terminal parses for them, modifiers riding only the editing, navigation and
// function keys; a key no terminal has is left to the system.
func TestKeysArriveAsATerminalReportsThem(t *testing.T) {
	key := func(k terminal.Key, mods terminal.Modifier) terminal.Event {
		return terminal.Event{Type: terminal.EventKey, Key: k, Modifiers: mods}
	}
	for _, c := range []struct {
		name       string
		code, meta int32
		char       rune
		want       terminal.Event
	}{
		{"back", 4, 0, 0, key(terminal.KeyEscape, 0)},
		{"enter", 66, 0, '\n', key(terminal.KeyEnter, 0)},
		{"ctrl enter", 66, metaCtrl, '\n', key(terminal.KeyEnter, 0)},
		{"shift up", 19, metaShift, 0, key(terminal.KeyUp, terminal.ModShift)},
		{"ctrl caret", 75, metaCtrl, '^', key(terminal.KeyCtrlCaret, 0)},
		{"backspace", 67, 0, 0, key(terminal.KeyBackspace, 0)},
		{"shift tab", 61, metaShift, '\t', key(terminal.KeyBacktab, terminal.ModShift)},
		{"ctrl c", 31, metaCtrl, 'c', key(terminal.KeyCtrlC, 0)},
		{"ctrl h", 36, metaCtrl, 'h', key(terminal.KeyBackspace, 0)},
		{"ctrl space", 62, metaCtrl, ' ', key(terminal.KeyCtrlSpace, 0)},
		{"shifted letter", 29, metaShift, 'A', terminal.Event{Type: terminal.EventKey, Key: terminal.KeyRune, Rune: 'A'}},
		{"alt letter", 29, metaAlt, 'a', terminal.Event{Type: terminal.EventKey, Key: terminal.KeyRune, Rune: 'a', Modifiers: terminal.ModAlt}},
		{"space", 62, 0, ' ', terminal.Event{Type: terminal.EventKey, Key: terminal.KeyRune, Rune: ' '}},
	} {
		if got, ok := keyEvent(c.code, c.char, c.meta); !ok || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.name, got, ok, c.want)
		}
	}
	if ev, ok := keyEvent(24, 0, 0); ok { // KEYCODE_VOLUME_UP
		t.Errorf("volume up was taken as %+v", ev)
	}
}

// TestTheBarTakesATapWithThePointerOff: with the program taking no pointer a
// press on the bar's rows is reported with its release, and one anywhere else is
// not; a release with no press taken is nothing.
func TestTheBarTakesATapWithThePointerOff(t *testing.T) {
	v := testView(false, 2)
	v.Resize(10*12, 4*24)
	if _, ok := v.touch(30, 30, true); ok {
		t.Fatal("a press above the bar was taken")
	}
	if _, ok := v.touch(30, 30, false); ok {
		t.Fatal("a release with no press was taken")
	}
	if ev, ok := v.touch(30, 4*24-1, true); !ok || ev.MouseY != 3 {
		t.Fatalf("a press on the bar: %+v %v", ev, ok)
	}
	if ev, ok := v.touch(30, 10, false); !ok || ev.MouseAction != terminal.MouseActionRelease {
		t.Fatalf("its release: %+v %v", ev, ok)
	}
}

// TestTheKeyboardHidesTheRowsUnderItAndTheStrip: the rows a program keeps clear
// are those reaching into the keyboard's pixels or the strip over them, the grid
// stretched over the view; no keyboard hides none and a whole one every row.
func TestTheKeyboardHidesTheRowsUnderItAndTheStrip(t *testing.T) {
	v := testView(false, 2.625)
	v.Resize(1008, 2244)
	_, rows := v.Size()
	row := 2244 / float64(rows)
	for _, px := range []int{0, 1, 900, 1300, 2244} {
		v.SetCovered(px)
		hidden := v.Covered()
		top := 2244 - float64(px) - stripRows*row // the strip's top edge
		switch {
		case px == 0 && hidden != 0, px == 2244 && hidden != rows:
			t.Fatalf("a keyboard of %d px hides %d of %d rows", px, hidden, rows)
		case px > 0 && px < 2244 && (float64(rows-hidden)*row > top+1e-9 || float64(rows-hidden+1)*row <= top):
			t.Fatalf("a keyboard of %d px hides %d of %d rows, the strip starting at %.1f px", px, hidden, rows, top)
		}
	}
}

// TestAPinchZoomsTheCellWithinBounds: a pinch scales the cell in the range's
// steps, and the grid the same pixels hold follows, posted as a resize where the
// view's first grid was not; it stops at Most, and short of Least at the font's
// smallest strike, which a 420 dpi phone reaches at 5 dp.
func TestAPinchZoomsTheCellWithinBounds(t *testing.T) {
	v := testView(false, 2.625)
	v.Resize(1008, 2244)
	cols, rows := v.Size()
	v.Zoom(2)
	c, r := v.Size()
	if v.Cell() != 2*DefaultCell || c*2 != cols && c*2 != cols-1 || r >= rows {
		t.Fatalf("at %g dp the grid is %dx%d, was %dx%d at %d", v.Cell(), c, r, cols, rows, DefaultCell)
	}
	if n := len(v.events); n != 1 || <-v.events != (terminal.Event{Type: terminal.EventResize, Width: c, Height: r}) {
		t.Fatalf("the first grid and the zoom posted %d events, want the zoom's resize to %dx%d alone", n, c, r)
	}
	if v.Zoom(10); v.Cell() != pinch.Most {
		t.Fatalf("zoomed in past the bound to %g dp", v.Cell())
	}
	v.Zoom(0.01)
	small := atlas.Hack().Strikes()[0]
	if v.Cell() != 5 || v.strike != small || v.cellW < small.Width || v.cellH < small.Height {
		t.Fatalf("zoomed out to %g dp, a %dx%d cell, where the smallest strike is %dx%d",
			v.Cell(), v.cellW, v.cellH, small.Width, small.Height)
	}
}

// TestAZoomAcrossAStrikeHandsTheDrawerItsTextureWithTheZoomedGrid: frames stay
// encoded for the texture the drawer holds until the program drew the zoomed grid;
// AtlasChanged then hands the new strike's over, once, and the next take is that
// grid's every cell as its drawer reads them, the raster's pixels too, so the grid
// is repainted once. A zoom within a strike hands nothing.
func TestAZoomAcrossAStrikeHandsTheDrawerItsTextureWithTheZoomedGrid(t *testing.T) {
	strikes := atlas.Hack().Strikes()
	small, face := strikes[0], strikes[len(strikes)-1]
	cells := make([]terminal.Cell, 4*2)
	for i := range cells {
		cells[i].Rune = rune('▖' + i) // quadrants: a glyph before them is blank in one strike alone
	}
	if a, b := glyph.NewTexture(small), glyph.NewTexture(face); a.Slot('▖') == b.Slot('▖') {
		t.Fatal("the strikes name the cells' glyphs by the same slots, which tells no frame of one from the other's")
	}
	for _, raster := range []bool{false, true} {
		v := testView(raster, 1) // 12 dp is the smallest strike's 12 pixels
		v.Resize(4*small.Width, 2*small.Height)
		// whole is every cell of a grid as the drawer holding a's texture reads them
		whole := func(a *glyph.Atlas, cols, rows int) []byte {
			if raster {
				return glyph.NewRaster(a).Frame(nil, cells[:cols*rows], cols, rows, nil)
			}
			return glyph.NewTexture(a).Frame(nil, cells[:cols*rows], cols, rows, nil)
		}
		holds := func(a *glyph.Atlas) bool { return bytes.Equal(v.Atlas(), glyph.NewTexture(a).Bytes()) }

		v.Flush(cells, 4, 2)
		if got := v.Take(); !bytes.Equal(got, whole(small, 4, 2)) || !holds(small) || v.AtlasChanged() != nil {
			t.Fatalf("raster %v: a view of %d px cells does not open on that strike", raster, small.Height)
		}
		v.Zoom(2) // the pixels now hold 2x1 cells of the face's strike
		v.Sync()
		v.Flush(cells, 4, 2)
		if v.AtlasChanged() != nil || !bytes.Equal(v.Take(), whole(small, 4, 2)) || !holds(small) {
			t.Fatalf("raster %v: the new strike was handed over before the program drew the zoomed grid", raster)
		}
		v.Flush(cells, 2, 1)
		if got := v.AtlasChanged(); !holds(face) || !bytes.Equal(got, v.Atlas()) || v.AtlasChanged() != nil {
			t.Fatalf("raster %v: a zoom to %d px cells did not hand over that strike once", raster, face.Height)
		}
		if got := v.Take(); !bytes.Equal(got, whole(face, 2, 1)) || v.Take() != nil {
			t.Fatalf("raster %v: the take after the hand-over is not the zoomed grid's every cell in the new strike, once", raster)
		}
		if v.Zoom(1.25); v.AtlasChanged() != nil || v.Take() != nil {
			t.Fatalf("raster %v: a zoom within the %d px strike changed the drawer's texture", raster, face.Height)
		}
	}
}
