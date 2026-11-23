//go:build linux || freebsd

package window

import (
	"bufio"
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
	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/atlas"
)

// TestAKeysymIsTheKeyOrTheCharacterItNames: a keysym is the non-printing key it
// names or the character of a Latin-1 or Unicode one, and neither for a modifier's
func TestAKeysymIsTheKeyOrTheCharacterItNames(t *testing.T) {
	for _, c := range []struct {
		keysym uint64
		key    terminal.Key
		r      rune
	}{
		{'a', terminal.KeyNone, 'a'}, {0xe9, terminal.KeyNone, 'é'}, {0x010003bb, terminal.KeyNone, 'λ'},
		{0xff0d, terminal.KeyEnter, 0}, {0xfe20, terminal.KeyBacktab, 0}, {0xff9f, terminal.KeyDelete, 0},
		{0xffe1, terminal.KeyNone, 0}, // Shift_L
	} {
		if key, r := keysymKeys[c.keysym], keysymRune(c.keysym); key != c.key || r != c.r {
			t.Errorf("keysym %#x is key %v, character %q; want %v, %q", c.keysym, key, r, c.key, c.r)
		}
	}
}

// TestCtrlWithAZoomKeyOrTheWheelZooms: Ctrl with =, + or the wheel up zooms in a
// step and with - or the wheel down out, the keypad's keys too, and Ctrl+0 puts the
// font's own cell back; no other key or wheel step zooms
func TestCtrlWithAZoomKeyOrTheWheelZooms(t *testing.T) {
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

// TestTheWindowOpensWithEachGLLibraryThatLoads: desktop OpenGL takes the context
// and the shaders GLES 2 takes, so the window opens on a system that has it and no
// GLES, as a Steam Linux Runtime
func TestTheWindowOpensWithEachGLLibraryThatLoads(t *testing.T) {
	xvfb(t)
	all := glLibs
	t.Cleanup(func() { glLibs, loaded.done = all, false }) // the next test binds the first again
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cfg := Config{Title: "test", Font: atlas.Hack(), Cols: 20, Rows: 10}
	for i := range all {
		glLibs, loaded.done = all[i:i+1], false
		if err := load(); err != nil {
			t.Logf("not on this system: %v", err)
			continue
		}
		w := newWindow(cfg)
		if err := w.open(cfg); err != nil {
			t.Errorf("%s: %v", all[i].path, err)
			continue
		}
		w.Flush(make([]terminal.Cell, cfg.Cols*cfg.Rows), cfg.Cols, cfg.Rows)
		w.present()
		w.release()
	}
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

// benchWindow opens the window a benchmark draws in, never waiting on the display's
// refresh, and how a frame is finished on the GPU. It needs an X display with EGL
// and GLES 2; under a compositor the swap still waits.
func benchWindow(b *testing.B, cfg Config) (*Window, func()) {
	if os.Getenv("DISPLAY") == "" {
		b.Skip("no X display")
	}
	if err := load(); err != nil {
		b.Skip(err)
	}
	w := newWindow(cfg)
	if err := w.open(cfg); err != nil {
		b.Skip(err)
	}
	eglSwapInterval(w.gpu.display, 0)
	return w, glFinish
}
