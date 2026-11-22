//go:build linux || freebsd

package window

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/atlas"
	"github.com/lixenwraith/terminal/glyph"
)

// TestAKeyPressReportsWhatATerminalReports: a keysym and its modifiers become the
// event the terminal module parses from the bytes a terminal sends for that key
func TestAKeyPressReportsWhatATerminalReports(t *testing.T) {
	const shift, ctrl, alt = stateShift, stateControl, stateMod1
	key := func(k terminal.Key, m terminal.Modifier) terminal.Event {
		return terminal.Event{Type: terminal.EventKey, Key: k, Modifiers: m}
	}
	char := func(r rune, m terminal.Modifier) terminal.Event {
		return terminal.Event{Type: terminal.EventKey, Key: terminal.KeyRune, Rune: r, Modifiers: m}
	}
	for _, c := range []struct {
		keysym uint64
		state  uint32
		want   terminal.Event
	}{
		{'a', 0, char('a', 0)},
		{'A', shift, char('A', 0)},
		{' ', 0, char(' ', 0)},
		{'x', alt, char('x', terminal.ModAlt)},
		{0x010003bb, 0, char('λ', 0)},
		{'c', ctrl, key(terminal.KeyCtrlC, 0)},
		{'C', ctrl | shift, key(terminal.KeyCtrlC, 0)},
		{'h', ctrl, key(terminal.KeyBackspace, 0)},
		{'[', ctrl, key(terminal.KeyEscape, 0)},
		{0xff0d, 0, key(terminal.KeyEnter, 0)},
		{0xff1b, 0, key(terminal.KeyEscape, 0)},
		{0xfe20, shift, key(terminal.KeyBacktab, terminal.ModShift)},
		{0xff52, shift, key(terminal.KeyUp, terminal.ModShift)},
		{0xffc9, ctrl | alt, key(terminal.KeyF12, terminal.ModCtrl|terminal.ModAlt)},
	} {
		if got, ok := keyEvent(c.keysym, c.state); !ok || got != c.want {
			t.Errorf("keysym %#x state %#x: %+v %v, want %+v", c.keysym, c.state, got, ok, c.want)
		}
	}
	if ev, ok := keyEvent(0xffe1, shift); ok { // Shift_L alone
		t.Errorf("a modifier alone reported %+v", ev)
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

// TestAZoomStepScalesTheWindowsCell: Ctrl with =, + or the wheel up zooms in a step
// and with - or the wheel down out, the keypad's keys too, within the range and
// never under the font's smallest strike, the grid the window's pixels hold
// following and the largest strike the cell holds drawing it; Ctrl+0 puts the
// font's own cell back, and no other key or wheel step zooms.
func TestAZoomStepScalesTheWindowsCell(t *testing.T) {
	const ctrl, in, out = stateControl, zoomStep, 1 / zoomStep
	for _, c := range []struct {
		keysym uint64
		state  uint32
		by     float64
		ok     bool
	}{
		{'=', ctrl, in, true}, {'+', ctrl | stateShift, in, true}, {0xffab, ctrl, in, true},
		{'-', ctrl, out, true}, {0xffad, ctrl, out, true}, {'0', ctrl, 0, true}, {0xffb0, ctrl, 0, true},
		{'=', 0, 0, false}, {'=', ctrl | stateMod1, 0, false}, {'a', ctrl, 0, false},
	} {
		if by, ok := zoomKey(c.keysym, c.state); by != c.by || ok != c.ok {
			t.Errorf("keysym %#x state %#x zooms by %v %v, want %v %v", c.keysym, c.state, by, ok, c.by, c.ok)
		}
	}
	for _, c := range []struct {
		button int32
		state  uint32
		by     float64
		ok     bool
	}{{4, ctrl, in, true}, {5, ctrl, out, true}, {4, 0, 0, false}, {1, ctrl, 0, false}} {
		if by, ok := zoomWheel(c.button, c.state); by != c.by || ok != c.ok {
			t.Errorf("button %d state %#x zooms by %v %v, want %v %v", c.button, c.state, by, ok, c.by, c.ok)
		}
	}

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

// TestAButtonReportsWhatATerminalReports: X's buttons 1 to 9 become the buttons SGR
// mouse reporting names; a wheel step reports its press alone, any other button
// its press and its release
func TestAButtonReportsWhatATerminalReports(t *testing.T) {
	w := newWindow(Config{Font: atlas.Hack(), Cols: 10, Rows: 5, Scale: 1})
	w.raster.Flush(make([]terminal.Cell, 10*5), 10, 5)
	w.winW, w.winH = 10*12, 5*24
	w.pointer.Store(pointerButtons)
	event := func(kind, button int32) *xEvent {
		var ev xEvent
		*(*int32)(unsafe.Pointer(&ev)) = kind
		*(*int32)(unsafe.Add(unsafe.Pointer(&ev), 84)) = button
		return &ev
	}
	wheel := []terminal.MouseButton{terminal.MouseBtnWheelUp, terminal.MouseBtnWheelDown, terminal.MouseBtnWheelLeft, terminal.MouseBtnWheelRight}
	for i, want := range []terminal.MouseButton{terminal.MouseBtnLeft, terminal.MouseBtnMiddle, terminal.MouseBtnRight,
		wheel[0], wheel[1], wheel[2], wheel[3], terminal.MouseBtnBack, terminal.MouseBtnForward} {
		button := int32(i + 1)
		if got, ok := w.mouseEvent(event(xButtonPress, button)); !ok || got.MouseBtn != want || got.MouseAction != terminal.MouseActionPress {
			t.Errorf("button %d pressed: %v %v %v, want a press of %v", button, got.MouseBtn, got.MouseAction, ok, want)
		}
		step := slices.Contains(wheel, want)
		if got, ok := w.mouseEvent(event(xButtonRelease, button)); ok == step || ok && got.MouseAction != terminal.MouseActionRelease {
			t.Errorf("button %d released: %v %v, a wheel step %v", button, got.MouseAction, ok, step)
		}
	}
	if ev, ok := w.mouseEvent(event(xButtonPress, 10)); ok {
		t.Errorf("button 10 reported %+v", ev)
	}
}

// TestAWindowOpensAtTheSizeItsManagerGivesIt: a window settles on the grid of the
// last size its manager gave and posts no resize for it, whether the manager sizes
// it before mapping it, after, or late for a state asked of it, and Open returns
// settled. The test is the manager, on an X server of its own.
func TestAWindowOpensAtTheSizeItsManagerGivesIt(t *testing.T) {
	xvfb(t)
	lib, err := purego.Dlopen(libX11, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip(err)
	}
	var xResizeWindow func(dpy, win uintptr, w, h uint32) int32
	var xSync func(dpy uintptr, discard int32) int32
	purego.RegisterLibFunc(&xResizeWindow, lib, "XResizeWindow")
	purego.RegisterLibFunc(&xSync, lib, "XSync")
	const xMapRequest, substructureRedirect = 20, 1 << 20
	// manage takes the next window that asks to be mapped and runs steps on it, those
	// past | once the caller has heard of the first; it holds the redirect on return
	manage := func(steps string) (acted, done chan struct{}) {
		ready := make(chan bool)
		acted, done = make(chan struct{}), make(chan struct{})
		go func() {
			defer close(done)
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			dpy := xOpenDisplay(0)
			if ready <- dpy != 0; dpy == 0 {
				return
			}
			defer xCloseDisplay(dpy)
			xSelectInput(dpy, xRootWindow(dpy, xDefaultScreen(dpy)), substructureRedirect)
			xSync(dpy, 0) // the server redirects from here
			ready <- true
			var ev xEvent
			for ev.int32At(0) != xMapRequest {
				xNextEvent(dpy, &ev)
			}
			win := uintptr(ev.uint64At(40))
			act := func(steps string) {
				for _, step := range strings.Fields(steps) {
					switch step {
					case "map":
						xMapWindow(dpy, win)
					case "half":
						xResizeWindow(dpy, win, 360, 360)
					default:
						xResizeWindow(dpy, win, 480, 480)
					}
				}
				xSync(dpy, 0) // the server has sent the window its events
			}
			first, late, _ := strings.Cut(steps, "|")
			act(first)
			close(acted)
			if late != "" {
				time.Sleep(2 * settleQuiet)
				act(late)
			}
		}()
		if !<-ready || !<-ready {
			t.Fatal("the manager reached no display")
		}
		return acted, done
	}
	given := func(name string, w *Window) {
		t.Helper()
		if cols, rows := w.Size(); cols != 40 || rows != 20 {
			t.Errorf("%s: settled at %dx%d, want the 40x20 its manager gave", name, cols, rows)
		}
		select {
		case ev := <-w.events:
			t.Errorf("%s: posted %+v for the size it opened at", name, ev)
		default:
		}
	}
	cfg := Config{Title: "test", Font: atlas.Hack(), Cols: 20, Rows: 10}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for name, c := range map[string]struct {
		asked bool
		steps string
	}{
		"sized before it is mapped":    {false, "size map"},
		"sized after it is mapped":     {false, "map size"},
		"sized again and mapped late":  {false, "half | size map"},
		"sized late for a state asked": {true, "map | size"},
	} {
		acted, done := manage(c.steps)
		cfg.Maximized = c.asked
		w := newWindow(cfg)
		if err := w.open(cfg); err != nil {
			t.Skip(err)
		}
		xFlush(w.dpy)
		<-acted // its events are sent before settle reads, so no clock decides the order
		start := time.Now()
		w.settle(c.asked)
		if took := time.Since(start); took < settleQuiet {
			t.Errorf("%s: settled in %v, before its size had held %v", name, took, settleQuiet)
		}
		given(name, w)
		<-done // before the window goes, which the manager may still be sizing
		w.release()
	}

	_, done := manage("size map")
	cfg.Maximized = false
	w, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	given("opened", w)
	<-done
	w.Close()
}

// xvfb starts an X server of the test's own, with no window manager, and makes it
// the display; a machine with none to start, or no libraries to draw with, skips
func xvfb(t *testing.T) {
	t.Helper()
	path, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("no Xvfb to open a window on")
	}
	if err := load(); err != nil {
		t.Skip(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	// -noreset: a server that starts over as its last client goes refuses the next
	cmd := exec.Command(path, "-displayfd", "3", "-noreset", "-screen", "0", "640x480x24", "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{w}
	err = cmd.Start()
	w.Close()
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGTERM) // it removes its socket and lock as it goes
		cmd.Wait()
	})
	r.SetReadDeadline(time.Now().Add(10 * time.Second))
	number, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		t.Skipf("Xvfb named no display: %v", err)
	}
	t.Setenv("DISPLAY", ":"+strings.TrimSpace(number))
	t.Setenv("LIBGL_ALWAYS_SOFTWARE", "1")
}

// BenchmarkPresent times a frame through the window: copy, raster, whole-texture
// upload, draw and swap, finished on the GPU, at 15 % and at every cell changed.
// Needs an X display with EGL and GLES 2; under a compositor the swap still waits
// for the display's refresh.
func BenchmarkPresent(b *testing.B) {
	if os.Getenv("DISPLAY") == "" {
		b.Skip("no X display")
	}
	if err := load(); err != nil {
		b.Skip(err)
	}
	for _, grid := range [][2]int{{120, 40}, {200, 60}} {
		for _, every := range []int{7, 1} {
			b.Run(fmt.Sprintf("%dx%d/changed=%d%%", grid[0], grid[1], 100/every), func(b *testing.B) {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				cfg := Config{Title: "bench", Font: atlas.Hack(), Cols: grid[0], Rows: grid[1]}
				w := newWindow(cfg)
				if err := w.open(cfg); err != nil {
					b.Skip(err)
				}
				defer w.release()
				eglSwapInterval(w.gpu.display, 0) // the work, not the display's refresh
				frames := gameFrames(grid[0]*grid[1], every)
				w.Flush(frames[0], grid[0], grid[1])
				w.present()
				b.ResetTimer()
				for i := range b.N {
					w.Flush(frames[(i+1)%2], grid[0], grid[1])
					w.present()
					glFinish()
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
