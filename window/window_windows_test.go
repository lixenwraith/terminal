package window

import (
	"runtime"
	"slices"
	"testing"
	"unsafe"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/atlas"
)

var (
	procGdiFlush         = gdi32.NewProc("GdiFlush")
	procDisableGhosting  = user32.NewProc("DisableProcessWindowsGhosting")
	procSetKeyboardState = user32.NewProc("SetKeyboardState")
)

// TestAKeyMessageReportsWhatATerminalReports: a key going down posts, with the
// characters queued behind it, what a terminal reports for the key, paired with
// its release; a chord of the window's own posts nothing, and text of no key going
// down is pressed and let go at once. The test is TranslateMessage: it queues the
// characters Windows would.
func TestAKeyMessageReportsWhatATerminalReports(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := load(); err != nil {
		t.Skip(err)
	}
	cfg := Config{Title: "test", Font: atlas.Hack(), Cols: 20, Rows: 10}
	w := newWindow(cfg)
	if err := w.open(cfg); err != nil {
		t.Skip(err)
	}
	defer w.release()
	w.keys.Ask(terminal.KeyReportReleases, terminal.KeySourceHost, true)

	hold := func(modifiers ...int) {
		var state [256]byte
		for _, vk := range modifiers {
			state[vk] = 0x80
		}
		procSetKeyboardState.Call(uintptr(unsafe.Pointer(&state)))
	}
	// down is a key going down at a scan code, the characters queued behind it
	// handled after it, as the window's loop would
	down := func(message uint32, vk, scan uintptr, chars ...rune) {
		for _, c := range chars {
			procPostMessageW.Call(w.hwnd, uintptr(message+wmChar-wmKeyDown), uintptr(c), scan<<16|1)
		}
		w.handle(message, vk, scan<<16|1)
		for m := (msg{}); ; w.handle(m.message, m.wParam, m.lParam) {
			if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), w.hwnd, wmChar, wmSysChar, 1); r == 0 {
				return
			}
		}
	}
	up := func(vk, scan uintptr) { w.handle(wmKeyUp, vk, scan<<16|0xC0000001) }
	posted := func() (keys []terminal.Event) {
		for {
			select {
			case ev := <-w.events:
				if ev.Type == terminal.EventKey {
					keys = append(keys, ev)
				}
			default:
				return keys
			}
		}
	}
	// held is a key's press and its release, lost for one that had no key
	held := func(k terminal.Key, r rune, m terminal.Modifier, lost bool) []terminal.Event {
		ev := terminal.Event{Type: terminal.EventKey, Key: k, Rune: r, Modifiers: m}
		release := ev
		release.Action, release.Lost = terminal.KeyRelease, lost
		return []terminal.Event{ev, release}
	}
	const shift, ctrl, alt, typed = vkShift, vkControl, vkMenu, terminal.KeyRune
	posted() // the answer to the ask
	for _, c := range []struct {
		name string
		keys func()
		want []terminal.Event
	}{
		{"a letter", func() { down(wmKeyDown, 'A', 0x1e, 'a'); up('A', 0x1e) }, held(typed, 'a', 0, false)},
		{"Shift with a letter", func() { hold(shift); down(wmKeyDown, 'A', 0x1e, 'A'); up('A', 0x1e) },
			held(typed, 'A', 0, false)},
		{"Enter, its character dropped", func() { down(wmKeyDown, vkReturn, 0x1c, '\r'); up(vkReturn, 0x1c) },
			held(terminal.KeyEnter, 0, 0, false)},
		{"Shift+Tab", func() { hold(shift); down(wmKeyDown, vkTab, 0x0f, '\t'); up(vkTab, 0x0f) },
			held(terminal.KeyBacktab, 0, terminal.ModShift, false)},
		{"Ctrl with a letter", func() { hold(ctrl); down(wmKeyDown, 'C', 0x2e, 3); up('C', 0x2e) },
			held(terminal.KeyCtrlC, 0, 0, false)},
		{"Ctrl+Space, its character dropped", func() { hold(ctrl); down(wmKeyDown, ' ', 0x39, ' '); up(' ', 0x39) },
			held(terminal.KeyCtrlSpace, 0, 0, false)},
		{"Ctrl+Alt with no character", func() { hold(ctrl, alt); down(wmKeyDown, 'X', 0x2d); up('X', 0x2d) },
			held(terminal.KeyCtrlX, 0, terminal.ModAlt, false)},
		{"AltGr with its character", func() { hold(ctrl, alt); down(wmKeyDown, 'Q', 0x10, '@'); up('Q', 0x10) },
			held(typed, '@', 0, false)},
		{"Alt with a letter", func() { hold(alt); down(wmSysKeyDown, 'X', 0x2d, 'x'); up('X', 0x2d) },
			held(typed, 'x', terminal.ModAlt, false)},
		{"a dead key, then the letter it marks", func() {
			procPostMessageW.Call(w.hwnd, wmDeadChar, '´', 0x0d<<16|1)
			down(wmKeyDown, 0xdd, 0x0d)
			up(0xdd, 0x0d)
			down(wmKeyDown, 'E', 0x12, 'é')
			up('E', 0x12)
		}, held(typed, 'é', 0, false)},
		{"a pair of surrogates", func() { down(wmKeyDown, 'E', 0x12, 0xd83d, 0xde00); up('E', 0x12) },
			held(typed, '😀', 0, false)},
		{"text of no key", func() { w.handle(wmChar, 'あ', 1) }, held(typed, 'あ', 0, true)},
		{"an input method's key", func() { down(wmKeyDown, vkProcessKey, 0x1e); up(vkProcessKey, 0x1e) }, nil},
		{"Ctrl with a zoom key", func() { hold(ctrl); down(wmKeyDown, vkOEMPlus, 0x0d); up(vkOEMPlus, 0x0d) }, nil},
		{"Ctrl-Shift-V, its character dropped", func() { hold(ctrl, shift); down(wmKeyDown, 'V', 0x2f, 0x16); up('V', 0x2f) },
			nil},
	} {
		hold()
		c.keys()
		if got := posted(); !slices.Equal(got, c.want) {
			t.Errorf("%s: posted %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestAWindowOpensHoldingTheGridOnScreen: Open returns with the window shown, its
// grid the one asked for or, full screen, the cells its monitor holds, and posts
// no resize for the size it opened at
func TestAWindowOpensHoldingTheGridOnScreen(t *testing.T) {
	for _, full := range []bool{false, true} {
		w, err := Open(Config{Title: "test", Font: atlas.Hack(), Cols: 20, Rows: 10, Fullscreen: full})
		if err != nil {
			t.Skip(err)
		}
		// Read on the test's thread: the window's own is idle until a message comes
		wantCols, wantRows := 20, 10
		if over := w.monitor().monitor; full {
			wantCols, wantRows = int(over.Right-over.Left)/w.cw, int(over.Bottom-over.Top)/w.ch
		}
		if cols, rows := w.Size(); cols != wantCols || rows != wantRows {
			t.Errorf("full screen %v: opened at %dx%d, want %dx%d", full, cols, rows, wantCols, wantRows)
		}
		select {
		case ev := <-w.Events():
			t.Errorf("full screen %v: posted %+v for the size it opened at", full, ev)
		default:
		}
		w.Close()
	}
}

// benchWindow opens the window a benchmark draws in: borderless, the grid's pixels
// at the font's own cell whatever the display's scale. A frame is finished once
// GDI has drawn it; a display that holds no window of that size skips.
func benchWindow(b *testing.B, cfg Config) (*Window, func()) {
	if err := load(); err != nil {
		b.Skip(err)
	}
	procDisableGhosting.Call() // a benchmark takes no messages, which the system reads as hung
	w := newWindow(cfg)
	if err := w.open(cfg); err != nil {
		b.Skip(err)
	}
	w.scale = float64(max(cfg.Scale, 1))
	w.rescaled(96)
	const wsVisible = 0x10000000
	width, height := cfg.Cols*w.cw, cfg.Rows*w.ch
	procSetWindowLongPtrW.Call(w.hwnd, gwlStyle, wsPopup|wsVisible)
	procSetWindowPos.Call(w.hwnd, 0, 0, 0, uintptr(width), uintptr(height), swpNoZOrder|swpFrameChanged)
	if w.winW != width || w.winH != height {
		w.release()
		b.Skipf("the display gave the window %dx%d of %dx%d pixels", w.winW, w.winH, width, height)
	}
	return w, func() { procGdiFlush.Call() }
}
