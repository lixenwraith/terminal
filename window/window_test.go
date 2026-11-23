//go:build linux || freebsd || windows

package window

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/atlas"
	"github.com/lixenwraith/terminal/glyph"
)

// TestAKeyPressReportsWhatATerminalReports: a non-printing key, or the character a
// key types under its modifiers, becomes the event the terminal module parses from
// the bytes a terminal sends for that key
func TestAKeyPressReportsWhatATerminalReports(t *testing.T) {
	const shift, alt, ctrl, none = terminal.ModShift, terminal.ModAlt, terminal.ModCtrl, terminal.KeyNone
	key := func(k terminal.Key, m terminal.Modifier) terminal.Event {
		return terminal.Event{Type: terminal.EventKey, Key: k, Modifiers: m}
	}
	char := func(r rune, m terminal.Modifier) terminal.Event {
		return terminal.Event{Type: terminal.EventKey, Key: terminal.KeyRune, Rune: r, Modifiers: m}
	}
	for _, c := range []struct {
		key  terminal.Key
		r    rune
		mods terminal.Modifier
		want terminal.Event
	}{
		{none, 'a', 0, char('a', 0)},
		{none, 'A', shift, char('A', 0)},
		{none, ' ', 0, char(' ', 0)},
		{none, 'x', alt, char('x', alt)},
		{none, 'λ', 0, char('λ', 0)},
		{none, 'c', ctrl, key(terminal.KeyCtrlC, 0)},
		{none, 'C', ctrl | shift, key(terminal.KeyCtrlC, 0)},
		{none, 'x', ctrl | alt, key(terminal.KeyCtrlX, alt)},
		{none, 'h', ctrl, key(terminal.KeyBackspace, 0)},
		{none, '[', ctrl, key(terminal.KeyEscape, 0)},
		{terminal.KeyEnter, 0, 0, key(terminal.KeyEnter, 0)},
		{terminal.KeyEscape, 0, 0, key(terminal.KeyEscape, 0)},
		{terminal.KeyBackspace, 0, ctrl, key(terminal.KeyBackspace, 0)},
		{terminal.KeyBacktab, 0, shift, key(terminal.KeyBacktab, shift)},
		{terminal.KeyUp, 0, shift, key(terminal.KeyUp, shift)},
		{terminal.KeyF12, 0, ctrl | alt, key(terminal.KeyF12, ctrl|alt)},
	} {
		if got, ok := keyEvent(c.key, c.r, c.mods); !ok || got != c.want {
			t.Errorf("key %v %q mods %#x: %+v %v, want %+v", c.key, c.r, c.mods, got, ok, c.want)
		}
	}
	for _, c := range []struct {
		r    rune
		mods terminal.Modifier
	}{{0, shift}, {'1', ctrl}} { // a modifier alone, and Ctrl with what sends no control byte
		if ev, ok := keyEvent(none, c.r, c.mods); ok {
			t.Errorf("%q mods %#x reported %+v", c.r, c.mods, ev)
		}
	}
}

// TestAPixelMapsToTheCellDrawnUnderIt: at scale 2 a cell covers twice the pixels of
// the font's largest strike each way, Hack's 12 by 24, the frame drawn centered,
// and a pixel in the margin takes the nearest edge cell
func TestAPixelMapsToTheCellDrawnUnderIt(t *testing.T) {
	w := newWindow(Config{Font: atlas.Hack(), Cols: 10, Rows: 5, Scale: 2})
	w.raster.Flush(make([]terminal.Cell, 10*5), 10, 5)
	w.winW, w.winH = 10*24+6, 5*48+4 // a margin of 3 pixels left and right, 2 above and below
	for _, c := range []struct{ px, py, x, y int }{
		{3, 2, 0, 0}, {3 + 24*4 + 23, 2 + 48*2 + 47, 4, 2}, {3 + 24*5, 2 + 48, 5, 1},
		{0, 0, 0, 0}, {w.winW - 1, w.winH - 1, 9, 4},
	} {
		if x, y := w.cellAt(c.px, c.py); x != c.x || y != c.y {
			t.Errorf("pixel %d,%d is cell %d,%d, want %d,%d", c.px, c.py, x, y, c.x, c.y)
		}
	}
}

// TestAZoomStepScalesTheWindowsCell: a zoom step scales the cell within the range
// and never under the font's smallest strike, the grid the window's pixels hold
// following and the largest strike the cell holds drawing it; a step of 0 puts the
// font's own cell back
func TestAZoomStepScalesTheWindowsCell(t *testing.T) {
	const in, out = zoomStep, 1 / zoomStep
	w := newWindow(Config{Font: atlas.Hack(), Cols: 40, Rows: 20, Scale: 2, Zoom: glyph.Zoom{Least: 4, Most: 40, Step: 0.5}})
	w.winW, w.winH = 40*24, 20*48
	for _, c := range []struct {
		by         float64
		moved      bool
		dp         float64
		cw, ch     int
		strike     int // the height of the strike whose raster draws the cell
		cols, rows int
	}{
		{in, true, 26.5, 27, 54, 24, 35, 17}, // from the font's own, 24 dp
		{100, true, 40, 40, 80, 24, 24, 12},  // to the most
		{in, false, 40, 40, 80, 24, 24, 12},  // and no further
		{0.25, true, 10, 10, 20, 16, 96, 48}, // under the largest strike's pixels, in the next
		{0.01, true, 6, 6, 12, 12, 160, 80},  // to the smallest strike's pixels, the least
		{out, false, 6, 6, 12, 12, 160, 80},  // and never under them
		{0, true, 0, 24, 48, 24, 40, 20},     // back to the font's own
		{0, false, 0, 24, 48, 24, 40, 20},    // where it already is
		{out, true, 22, 22, 44, 24, 43, 21},  // a step out, pixels the largest strike still fits
	} {
		moved := w.rescale(c.by)
		w.regrid()
		cols, rows := w.Size()
		w.raster.Flush(make([]terminal.Cell, 1), 1, 1)
		_, strike := w.raster.Bounds()
		if moved != c.moved || w.Cell() != c.dp || w.cw != c.cw || w.ch != c.ch || strike != c.strike || cols != c.cols || rows != c.rows {
			t.Fatalf("a step of %v: moved %v to %v dp, a %dx%d cell of the strike %d high, a %dx%d grid; want %v, %v dp, %dx%d of %d, %dx%d",
				c.by, moved, w.Cell(), w.cw, w.ch, strike, cols, rows, c.moved, c.dp, c.cw, c.ch, c.strike, c.cols, c.rows)
		}
	}
}

// BenchmarkPresent times a frame through the window: copy, raster and the host's
// present, finished before the next frame, at 15 % and at every cell changed. Its
// shapes are a window's pixels and its cell, the strike's own 12x24 or that
// stretched: by 2, each pixel a block, or by 1.5, filtered.
func BenchmarkPresent(b *testing.B) {
	for _, shape := range []struct {
		name string
		cfg  Config
	}{
		{"1920x1080/cell=12x24", Config{Cols: 160, Rows: 45}},
		{"3840x2160/cell=12x24", Config{Cols: 320, Rows: 90}},
		{"3840x2160/cell=24x48", Config{Cols: 160, Rows: 45, Scale: 2}},
		{"1908x1080/cell=18x36", Config{Cols: 106, Rows: 30, Cell: 36}},
		{"3834x2160/cell=18x36", Config{Cols: 213, Rows: 60, Cell: 36}},
	} {
		for _, every := range []int{7, 1} {
			b.Run(fmt.Sprintf("%s/changed=%d%%", shape.name, 100/every), func(b *testing.B) {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				cfg := shape.cfg
				cfg.Title, cfg.Font = "bench", atlas.Hack()
				w, finish := benchWindow(b, cfg)
				defer w.release()
				frames := gameFrames(cfg.Cols*cfg.Rows, every)
				w.Flush(frames[0], cfg.Cols, cfg.Rows)
				w.present()
				b.ResetTimer()
				for i := range b.N {
					w.Flush(frames[(i+1)%2], cfg.Cols, cfg.Rows)
					w.present()
					finish()
				}
			})
		}
	}
}

// gameFrames are two frames where every cell has a glyph and a background, as a
// game frame does, differing in one cell of every
func gameFrames(n, every int) [2][]terminal.Cell {
	frames := [2][]terminal.Cell{make([]terminal.Cell, n), make([]terminal.Cell, n)}
	for i := range n {
		frames[0][i] = terminal.Cell{Rune: rune('!' + i%90), Fg: color.RGB{R: uint8(i), G: 200}, Bg: color.RGB{B: uint8(i / 7)}}
		frames[1][i] = frames[0][i]
		if i%every == 0 {
			frames[1][i].Rune, frames[1][i].Fg = '#', color.RGB{G: uint8(i)}
		}
	}
	return frames
}
