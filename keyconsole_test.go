package terminal

import (
	"strings"
	"testing"
	"time"
)

// fakeConsole is a backend on a Linux console: its keyboard goes raw when
// asked, unless it refuses, and raw, its input is K_MEDIUMRAW keycodes typed
// by the kernel's built-in keymap
type fakeConsole struct {
	fakeBackend
	refuse   bool
	isRaw    bool
	away     bool
	prepares int
	rawAt    int // how much output the terminal had written when the keyboard went raw
	cookedAt int // and when it went back
	closed   bool
	kb       linuxKeys
	dec      mediumRaw
}

func newFakeConsole(t *testing.T) *fakeConsole {
	return &fakeConsole{kb: linuxKeys{km: testKeymap(t)}}
}

func (f *fakeConsole) keySources() KeySource  { return KeySourceConsole | KeySourceProtocol }
func (f *fakeConsole) keyConsole() keyConsole { return f }
func (f *fakeConsole) Fini()                  { f.close() } // as unixBackend's

func (f *fakeConsole) prepare() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prepares++
	return !f.refuse
}

func (f *fakeConsole) raw() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isRaw, f.rawAt = true, f.out.Len()
	f.kb.seed(0, 0)
	return true
}

func (f *fakeConsole) cooked() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.isRaw {
		f.isRaw, f.cookedAt = false, f.out.Len()
	}
}

func (f *fakeConsole) keys(data []byte, key func(code uint32, action KeyAction, typed []byte)) {
	f.dec.decode(data, func(code int, down bool) {
		action := KeyRelease
		if down {
			action = KeyPress
			if f.kb.isDown(code) {
				action = KeyRepeat
			}
		}
		typed, _ := f.kb.key(code, down, action == KeyRepeat)
		key(uint32(code), action, typed)
	})
}

func (f *fakeConsole) front() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.away
}

func (f *fakeConsole) close() {
	f.cooked()
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

// keyed hands the reader keycodes, as the raw console sends them
func (f *fakeConsole) keyed(codes ...byte) {
	f.mu.Lock()
	f.chunks = append(f.chunks, string(codes))
	f.mu.Unlock()
}

// state reads the fake's fields under its lock
func (f *fakeConsole) state() (isRaw bool, prepares int, out string, rawAt, cookedAt int, closed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isRaw, f.prepares, f.out.String(), f.rawAt, f.cookedAt, f.closed
}

// up is a keycode's release byte
func up(code byte) byte { return code | 0x80 }

// keyEvent is a key event as the console source sends it
func keyEvent(action KeyAction, r rune, lost bool) Event {
	return Event{Type: EventKey, Key: KeyRune, Rune: r, Action: action, Lost: lost}
}

// The console's keyboard goes raw when releases are asked of it, before the
// protocol. Raw, each key types as it does cooked and is held by its keycode:
// Shift let go first still releases 'A', a key held repeats, and another
// console taking the keyboard ends the holds Lost. Mouse reports are off while
// the keyboard is raw, the console having taken that first, and come back on
// with it. Going back to presses switches the keyboard back, and so does the
// input ending at Fini, before the backend closes the console.
func TestConsoleSourceComesAndGoes(t *testing.T) {
	f := newFakeConsole(t)
	term := newTerm(f, ColorModeNone)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	term.SetMouseMode(MouseModeClick)

	term.SetKeyReport(KeyReportReleases, KeySourceConsole|KeySourceProtocol)
	if ev := poll(t, term); ev.Type != EventKeyReport || ev.Report != KeyReportReleases || ev.Source != KeySourceConsole {
		t.Fatalf("answered %+v, want releases from the console", ev)
	}
	isRaw, _, out, rawAt, _, _ := f.state()
	if !isRaw {
		t.Fatal("keyboard not raw")
	}
	if !strings.Contains(out[:rawAt], string(csiMouseClickOff)) {
		t.Errorf("mouse reports still on as the keyboard went raw: %q", out[:rawAt])
	}

	f.keyed(keyLeftShift, keyA, up(keyLeftShift), up(keyA))
	f.keyed(keyQ, keyQ, up(keyQ))
	f.keyed(keyEnter, up(keyEnter))
	want := []Event{
		keyEvent(KeyPress, 'A', false), keyEvent(KeyRelease, 'A', false),
		keyEvent(KeyPress, 'q', false), keyEvent(KeyRepeat, 'q', false), keyEvent(KeyRelease, 'q', false),
		{Type: EventKey, Key: KeyEnter, Action: KeyPress}, {Type: EventKey, Key: KeyEnter, Action: KeyRelease},
	}
	for i, w := range want {
		if ev := poll(t, term); ev != w {
			t.Fatalf("event %d: %+v, want %+v", i, ev, w)
		}
	}

	f.keyed(keyA)
	if ev := poll(t, term); ev != keyEvent(KeyPress, 'a', false) {
		t.Fatalf("got %+v, want a press of 'a'", ev)
	}
	f.mu.Lock()
	f.away = true
	f.mu.Unlock()
	if ev := poll(t, term); ev != keyEvent(KeyRelease, 'a', true) {
		t.Fatalf("got %+v, want 'a' released Lost", ev)
	}
	f.mu.Lock()
	f.away = false
	f.mu.Unlock()

	term.SetKeyReport(KeyReportPresses, 0)
	if ev := poll(t, term); ev.Type != EventKeyReport || ev.Report != KeyReportPresses || ev.Source != 0 {
		t.Fatalf("answered %+v, want presses", ev)
	}
	waitFor(t, "mouse reports back on", func() bool {
		isRaw, _, out, _, cookedAt, _ := f.state()
		return !isRaw && strings.Contains(out[cookedAt:], string(csiMouseClickOn))
	})

	term.SetKeyReport(KeyReportReleases, KeySourceConsole)
	poll(t, term)
	f.mu.Lock()
	f.closed = false
	f.mu.Unlock()
	term.input.stop()
	if isRaw, _, _, _, _, closed := f.state(); isRaw || closed {
		t.Errorf("input ended with the keyboard raw %v, the console closed %v", isRaw, closed)
	}
	term.Fini()
	if _, _, _, _, _, closed := f.state(); !closed {
		t.Error("Fini left the console open")
	}
}

// A console that can't go raw is not asked again: the protocol is next, and
// the console here doesn't answer its questions
func TestConsoleRefusingIsAskedOnce(t *testing.T) {
	f := newFakeConsole(t)
	f.refuse = true
	old := protoTimeout
	protoTimeout = 50 * time.Millisecond
	defer func() { protoTimeout = old }()
	term := newTerm(f, ColorModeNone)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	for range 2 {
		term.SetKeyReport(KeyReportReleases, KeySourceConsole|KeySourceProtocol)
		if ev := poll(t, term); ev.Type != EventKeyReport || ev.Report != KeyReportPresses {
			t.Fatalf("answered %+v, want presses", ev)
		}
	}
	if _, prepares, _, _, _, _ := f.state(); prepares != 1 {
		t.Errorf("console asked %d times, want once", prepares)
	}
}

// Mouse reporting asked for while the keyboard is raw starts once it is
// cooked again
func TestConsoleHoldsMouseReportsWhileRaw(t *testing.T) {
	f := newFakeConsole(t)
	term := newTerm(f, ColorModeNone)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	term.SetKeyReport(KeyReportReleases, KeySourceConsole)
	poll(t, term)
	term.SetMouseMode(MouseModeClick | MouseModeDrag)
	if _, _, out, rawAt, _, _ := f.state(); strings.Contains(out[rawAt:], string(csiMouseSGROn)) {
		t.Fatalf("mouse reports on while raw: %q", out[rawAt:])
	}
	term.SetKeyReport(KeyReportPresses, 0)
	poll(t, term)
	waitFor(t, "mouse reports on", func() bool {
		_, _, out, _, cookedAt, _ := f.state()
		return strings.Contains(out[cookedAt:], string(csiMouseDragOn))
	})
}

// A key whose string reads as several keys taps all but the last, which it
// holds by its keycode
func TestConsoleKeyTypingSeveralKeys(t *testing.T) {
	f := newFakeConsole(t)
	f.kb.km.funcs[0] = "ab" // F1
	term := newTerm(f, ColorModeNone)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	term.SetKeyReport(KeyReportReleases, KeySourceConsole)
	poll(t, term)
	f.keyed(keyF1, up(keyF1))
	want := []Event{
		keyEvent(KeyPress, 'a', false), keyEvent(KeyRelease, 'a', false), keyEvent(KeyPress, 'b', false),
		keyEvent(KeyRelease, 'b', false),
	}
	for i, w := range want {
		if ev := poll(t, term); ev != w {
			t.Fatalf("event %d: %+v, want %+v", i, ev, w)
		}
	}
}
