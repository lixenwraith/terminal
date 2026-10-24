package terminal

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lixenwraith/color"
)

// fakeBackend records output and replays input chunks; an empty chunk, or
// none left, is a poll timing out, and pause blocks a read as wasm's does
type fakeBackend struct {
	mu     sync.Mutex
	out    bytes.Buffer
	chunks []string
	pause  time.Duration
	gate   chan struct{} // when set, a write waits until it is closed
}

func (f *fakeBackend) Init() error                              { return nil }
func (f *fakeBackend) Fini()                                    {}
func (f *fakeBackend) Size() (int, int)                         { return 1, 1 }
func (f *fakeBackend) SetResizeHandler(func(width, height int)) {}

func (f *fakeBackend) Write(p []byte) error {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out.Write(p)
	return nil
}

func (f *fakeBackend) Read(stop <-chan struct{}) ([]byte, error) {
	f.mu.Lock()
	if len(f.chunks) > 0 {
		c := f.chunks[0]
		f.chunks = f.chunks[1:]
		f.mu.Unlock()
		if c == "" {
			time.Sleep(5 * time.Millisecond) // a poll timing out
		}
		time.Sleep(f.pause)
		return []byte(c), nil
	}
	f.mu.Unlock()
	select {
	case <-stop:
	case <-time.After(5 * time.Millisecond):
	}
	return nil, nil
}

func (f *fakeBackend) written() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
}

// Each color mode writes each kind of color as such a terminal reads it:
// 16 colors as SGR 30-37/90-97 and 40-47/100-107, restated from a reset
// since a console's bright colors set bold, a palette index by its shade,
// the terminal's own as 39/49 in every mode, no color at all in
// ColorModeNone, and elsewhere only what changed from one cell to the next
func TestColorModesWriteWhatTheTerminalReads(t *testing.T) {
	red, yellow := color.RGB{R: 205}, color.RGB{R: 255, G: 255}
	for _, c := range []struct {
		mode  ColorMode
		cells []Cell
		want  string
	}{
		{ColorMode16, []Cell{{Rune: 'x', Fg: red, Bg: yellow}}, "\x1b[0;31;103mx"},
		{ColorMode16, []Cell{{Rune: 'x', Fg: color.RGB{R: 9}, Bg: color.RGB{R: 196}, Attrs: AttrFg256 | AttrBg256}}, "\x1b[0;91;101mx"},
		{ColorMode16, []Cell{{Rune: 'x', Fg: color.RGB{R: 244}, Attrs: AttrFg256 | AttrBgDefault | AttrBold}}, "\x1b[0;1;90;49mx"},
		{ColorMode16, []Cell{{Rune: 'x', Fg: color.RGB{R: 9}, Attrs: AttrFg256}, {Rune: 'y', Fg: color.RGB{R: 1}, Attrs: AttrFg256}}, "\x1b[0;91;40mx\x1b[0;31;40my"},
		{ColorMode256, []Cell{{Rune: 'x', Attrs: AttrFgDefault | AttrBgDefault}}, "\x1b[0;39;49mx"},
		{ColorMode256, []Cell{{Rune: 'x', Fg: color.RGB{R: 1}, Attrs: AttrFg256}, {Rune: 'y', Fg: color.RGB{R: 1}, Bg: color.RGB{R: 2}, Attrs: AttrFg256 | AttrBg256},
			{Rune: 'z', Fg: color.RGB{R: 3}, Bg: color.RGB{R: 4}, Attrs: AttrFg256 | AttrBg256}}, "\x1b[0;38;5;1;48;5;16mx\x1b[48;5;2my\x1b[38;5;3;48;5;4mz"},
		{ColorModeTrueColor, []Cell{{Rune: 'x', Fg: color.RGB{R: 1, G: 2, B: 3}, Attrs: AttrBgDefault}}, "\x1b[0;38;2;1;2;3;49mx"},
		{ColorModeNone, []Cell{{Rune: 'x', Fg: red, Bg: yellow, Attrs: AttrFg256 | AttrUnderline}, {Rune: 'y', Fg: yellow, Attrs: AttrUnderline}}, "\x1b[0;4mxy"},
	} {
		o := newOutputBuffer(c.mode)
		o.flush(c.cells, len(c.cells), 1)
		if got, want := o.buf.String(), "\x1b[1;1H"+c.want+"\x1b[0m"; got != want {
			t.Errorf("mode %d %+v: %q, want %q", c.mode, c.cells, got, want)
		}
	}
}

// A cell is drawn as one glyph: a control character, C0 or C1, a format one
// (a bidi control, a zero-width space) or a line separator that a pasted
// value or a log line carried in, or no rune at all, is drawn as a space,
// never sent to the terminal
func TestCellsSendNoControlCharacters(t *testing.T) {
	o := newOutputBuffer(ColorMode256)
	o.flush([]Cell{{Rune: 0x1b}, {Rune: 0x9b}, {Rune: 0x7f}, {Rune: -229}, {Rune: 0xd800}, {Rune: 0x202e}, {Rune: 0x2067}, {Rune: 0x061c}, {Rune: 0x200b}, {Rune: 0x2028}, {Rune: 'é'}}, 11, 1)
	if got, want := o.buf.String(), "\x1b[1;1H\x1b[0;38;5;16;48;5;16m          é\x1b[0m"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
}

// A cell is one column: a rune a terminal draws in none (a combining mark, a
// joining jamo), in two (wide, fullwidth, emoji) or in its neighbour's cluster
// (a spacing mark, a letter such as Thai's sara am) is drawn as U+FFFD, any
// other as itself
func TestEachCellIsOneColumn(t *testing.T) {
	o := newOutputBuffer(ColorMode256)
	var row []Cell
	for _, r := range "\u0301\u20dd\u1161漢Ａ😀✅\U0001f1fa\u093e\u0e33é─❤" {
		row = append(row, Cell{Rune: r})
	}
	o.flush(row, len(row), 1)
	if got, want := o.buf.String(), "\x1b[1;1H\x1b[0;38;5;16;48;5;16m"+strings.Repeat("\ufffd", 10)+"é─❤\x1b[0m"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
}

// A frame that changes no cell writes nothing, so drawing an idle screen
// again costs the terminal no bytes
func TestUnchangedFrameWritesNothing(t *testing.T) {
	o := newOutputBuffer(ColorMode256)
	frame := []Cell{{Rune: 'a'}, {Rune: 'é'}}
	o.flush(frame, len(frame), 1)
	o.buf.Reset()
	if o.flush(frame, len(frame), 1); o.buf.Len() != 0 {
		t.Fatalf("unchanged frame wrote %q", o.buf.String())
	}
}

// A paste arrives as one event, whatever reads split it into: an escape
// inside it is text even when a poll times out on it, and a paste the
// terminal never ends ends when input pauses, whether reads time out or
// block. With paste mode off, a paste marker is no paste.
func TestPasteArrivesWhole(t *testing.T) {
	defer func(d time.Duration) { pasteIdle = d }(pasteIdle)
	paste := func(s string) Event { return Event{Type: EventPaste, Text: s} }
	expectEvents(t, map[string]input{
		"split":    {chunks: []string{"a\x1b[20", "0~", "\x1b", "", "[A one\ntwo\x1b[2", "01~b"}, want: []Event{key('a'), paste("\x1b[A one\ntwo"), key('b')}},
		"unended":  {idle: 20 * time.Millisecond, chunks: []string{"\x1b[200~ab\x1b[2", "", "", "", "", "", "", "", "", "", "q"}, want: []Event{paste("ab\x1b[2"), key('q')}},
		"blocking": {idle: 20 * time.Millisecond, pause: 40 * time.Millisecond, chunks: []string{"\x1b[200~ab", "q"}, want: []Event{paste("ab"), key('q')}},
		"mode off": {off: true, chunks: []string{"\x1b[200~q"}, want: []Event{key('q')}},
	})
}

// ESC ESC is Escape before a paste, in one read or split after the second
// ESC; before another sequence it is Alt and that key (rxvt's Alt+arrows);
// alone, once input pauses, Alt+Escape
func TestDoubleEscape(t *testing.T) {
	esc := Event{Type: EventKey, Key: KeyEscape}
	altEsc := Event{Type: EventKey, Key: KeyEscape, Modifiers: ModAlt}
	pasted := Event{Type: EventPaste, Text: "hi"}
	expectEvents(t, map[string]input{
		"one read": {chunks: []string{"\x1b\x1b[200~hi\x1b[201~"}, want: []Event{esc, pasted}},
		"split":    {chunks: []string{"\x1b\x1b", "[200~hi\x1b[201~"}, want: []Event{esc, pasted}},
		"alt+up":   {chunks: []string{"\x1b\x1b[A"}, want: []Event{{Type: EventKey, Key: KeyUp, Modifiers: ModAlt}}},
		"alone":    {chunks: []string{"\x1b\x1b"}, want: []Event{altEsc}},
	})
}

func key(r rune) Event { return Event{Type: EventKey, Key: KeyRune, Rune: r} }

// parsed is what a reader sends for input arriving in chunks without pauses,
// and how many bytes it still holds back for the rest of a sequence
func parsed(chunks ...string) (events []Event, held int) {
	r := &inputReader{eventCh: make(chan Event, 4096)}
	r.pasteMode.Store(true)
	var buf []byte
	for _, c := range chunks {
		buf = append(buf, c...)
		buf = buf[r.parseInput(buf):]
	}
	close(r.eventCh)
	for ev := range r.eventCh {
		events = append(events, ev)
	}
	return events, len(buf)
}

// Every sequence the key tables hold reads as its key, and the key after it
// as itself
func TestKnownSequencesKeepTheirKeys(t *testing.T) {
	for _, s := range csiSequences {
		want := []Event{{Type: EventKey, Key: s.key, Modifiers: s.mod}, key('x')}
		if got, _ := parsed("\x1b[" + s.seq + "x"); !slices.Equal(got, want) {
			t.Errorf("ESC [ %s: got %+v, want %+v", s.seq, got, want)
		}
	}
}

// No input stops the reader: a reply of any length, Alt with Backspace or a
// character past ASCII, a malformed mouse report, or ESC [ before a byte no
// sequence holds (typed Alt+[) leave the keys after them read. An unknown or
// over-long sequence is dropped up to its final byte, by ECMA-48.
func TestNoInputStopsTheReader(t *testing.T) {
	altBracket := Event{Type: EventKey, Key: KeyRune, Rune: '[', Modifiers: ModAlt}
	enter := Event{Type: EventKey, Key: KeyEnter}
	for _, c := range []struct {
		name   string
		chunks []string
		want   []Event
	}{
		{"long reply", []string{"\x1b[?65;1;9;15;18;21;22cx"}, []Event{key('x')}},
		{"split reply", []string{"\x1b[?65;1;9;", "15;18;21;22", "cx"}, []Event{key('x')}},
		{"longest reply", []string{"\x1b[" + strings.Repeat("1", csiMaxLen-3) + "zx"}, []Event{key('x')}},
		{"over-long reply", []string{"\x1b[" + strings.Repeat("1", csiMaxLen) + ";2", "3zx"}, []Event{key('x')}},
		{"over-long cut", []string{"\x1b[" + strings.Repeat("1", csiMaxLen) + "\rx"}, []Event{enter, key('x')}},
		{"unknown final", []string{"\x1b[2^x"}, []Event{key('x')}},
		{"alt+backspace", []string{"\x1b\x7fx"}, []Event{{Type: EventKey, Key: KeyBackspace, Modifiers: ModAlt}, key('x')}},
		{"alt+ö", []string{"\x1b\xc3", "\xb6x"}, []Event{{Type: EventKey, Key: KeyRune, Rune: 'ö', Modifiers: ModAlt}, key('x')}},
		{"stray byte", []string{"\x1b\x80x"}, []Event{{Type: EventKey, Key: KeyEscape}, key('x')}},
		{"alt+[ enter", []string{"\x1b[\rx"}, []Event{altBracket, enter, key('x')}},
		{"alt+[ 1 é", []string{"\x1b[1éx"}, []Event{altBracket, key('1'), key('é'), key('x')}},
		{"esc alt+[ enter", []string{"\x1b\x1b[\rx"}, []Event{{Type: EventKey, Key: KeyEscape, Modifiers: ModAlt}, key('['), enter, key('x')}},
		{"bad mouse", []string{"\x1b[<0;1;1;1Mx"}, []Event{key('x')}},
		{"mouse", []string{"\x1b[<0;5;3M\x1b[<0;5;3m"}, []Event{
			{Type: EventMouse, MouseX: 4, MouseY: 2, MouseBtn: MouseBtnLeft, MouseAction: MouseActionPress},
			{Type: EventMouse, MouseX: 4, MouseY: 2, MouseBtn: MouseBtnLeft, MouseAction: MouseActionRelease}}},
	} {
		if got, held := parsed(c.chunks...); !slices.Equal(got, c.want) || held != 0 {
			t.Errorf("%s: got %+v holding %d, want %+v", c.name, got, held, c.want)
		}
	}
}

// Whatever bytes arrive, however reads split them, the reader sends the same
// events, holds back no more than ESC and the longest sequence, and holds
// nothing once four DEL, which end any sequence or character, and a paste
// end follow
func FuzzInputNeverStalls(f *testing.F) {
	for _, s := range []string{"\x1b[?65;1;9;15;18;21;22cx", "\x1b\x7fx", "\x1b\xc3\xb6", "\x1b[\rx", "\x1b[<0;1;1;1Mx", "\x1b\x1b[A", "a\x1b[200~\x1b[A\x1b[201~b"} {
		f.Add(s, uint(len(s)/2))
	}
	f.Fuzz(func(t *testing.T, in string, cut uint) {
		cut %= uint(len(in)) + 1
		whole, held := parsed(in)
		if split, _ := parsed(in[:cut], in[cut:]); !slices.Equal(whole, split) {
			t.Fatalf("%q: whole %+v, split at %d %+v", in, whole, cut, split)
		}
		if held > csiMaxLen+1 {
			t.Fatalf("%q: holds %d bytes", in, held)
		}
		if _, held := parsed(in, "\x7f\x7f\x7f\x7f\x1b[201~"); held != 0 {
			t.Fatalf("%q: stalled holding %d bytes", in, held)
		}
	})
}

// input is what a reader is given, in chunks, and the events it must send
type input struct {
	off         bool
	idle, pause time.Duration
	chunks      []string
	want        []Event
}

func expectEvents(t *testing.T, cases map[string]input) {
	t.Helper()
	for name, c := range cases {
		pasteIdle = cmp.Or(c.idle, time.Second)
		r := newInputReader(&fakeBackend{chunks: c.chunks, pause: c.pause})
		r.pasteMode.Store(!c.off)
		r.start()
		for _, w := range c.want {
			select {
			case ev := <-r.events():
				if ev != w {
					t.Errorf("%s: got %+v, want %+v", name, ev, w)
				}
			case <-time.After(2 * time.Second):
				t.Errorf("%s: no event, want %+v", name, w)
			}
		}
		r.stop()
	}
}

// Paste mode set on the terminal is what its reader parses by
func TestPasteModeReachesTheReader(t *testing.T) {
	f := &fakeBackend{}
	term := newTerm(f, ColorMode256)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	term.SetPasteMode(true)
	f.mu.Lock()
	f.chunks = append(f.chunks, "\x1b[200~x\x1b[201~")
	f.mu.Unlock()
	got := make(chan Event, 1)
	go func() { got <- term.PollEvent() }()
	select {
	case ev := <-got:
		if ev.Type != EventPaste || ev.Text != "x" {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
}

// The terminal's own bidi reordering is off while the session runs, paste
// mode ends with it, and Init clears to the terminal's own background rather
// than painting it black
func TestSessionLeavesTheTerminalAsItFoundIt(t *testing.T) {
	f := &fakeBackend{}
	term := newTerm(f, ColorModeTrueColor)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	if err := term.SetPasteMode(true); err != nil {
		t.Fatal(err)
	}
	term.Fini()
	got := f.written()
	if !strings.Contains(got, "\x1b[0m\x1b[49m\x1b[2J") || !strings.Contains(got, "\x1b[?2004h") ||
		!strings.Contains(got[strings.Index(got, "\x1b[?2004h"):], "\x1b[?2004l") {
		t.Fatalf("%q", got)
	}
	if i := strings.Index(got, "\x1b[8l"); i < 0 || !strings.Contains(got[i:], "\x1b[8h") {
		t.Fatalf("bidi reordering not off for the session: %q", got)
	}
}

// A mouse mode change writes the modes dropped as reset and the modes added
// as set, and sets the highest mode kept again, last, when another was dropped
// or added under it; SGR is on while any mode is. Whether the terminal keeps
// one mode (xterm, xterm.js, kitty), lets a set clear the others (alacritty)
// or keeps each and reports by the highest (VTE, WezTerm, Windows Terminal),
// it then reports by the highest mode asked for.
func TestMouseModeChangesKeepReporting(t *testing.T) {
	c, d, m := MouseModeClick, MouseModeDrag, MouseModeMotion
	for _, tc := range []struct {
		from, to MouseMode
		want     string // +n sets mode n, -n resets it
	}{
		{0, 0, ""}, {0, c, "+1006 +1000"}, {0, d, "+1006 +1002"}, {0, c | d, "+1006 +1000 +1002"},
		{0, m, "+1006 +1003"}, {0, c | m, "+1006 +1000 +1003"}, {0, d | m, "+1006 +1002 +1003"}, {0, c | d | m, "+1006 +1000 +1002 +1003"},
		{c, 0, "-1000 -1006"}, {c, c, ""}, {c, d, "-1000 +1002"}, {c, c | d, "+1002"},
		{c, m, "-1000 +1003"}, {c, c | m, "+1003"}, {c, d | m, "-1000 +1002 +1003"}, {c, c | d | m, "+1002 +1003"},
		{d, 0, "-1002 -1006"}, {d, c, "-1002 +1000"}, {d, d, ""}, {d, c | d, "+1000 +1002"},
		{d, m, "-1002 +1003"}, {d, c | m, "-1002 +1000 +1003"}, {d, d | m, "+1003"}, {d, c | d | m, "+1000 +1003"},
		{c | d, 0, "-1002 -1000 -1006"}, {c | d, c, "-1002 +1000"}, {c | d, d, "-1000 +1002"}, {c | d, c | d, ""},
		{c | d, m, "-1002 -1000 +1003"}, {c | d, c | m, "-1002 +1003"}, {c | d, d | m, "-1000 +1003"}, {c | d, c | d | m, "+1003"},
		{m, 0, "-1003 -1006"}, {m, c, "-1003 +1000"}, {m, d, "-1003 +1002"}, {m, c | d, "-1003 +1000 +1002"},
		{m, m, ""}, {m, c | m, "+1000 +1003"}, {m, d | m, "+1002 +1003"}, {m, c | d | m, "+1000 +1002 +1003"},
		{c | m, 0, "-1003 -1000 -1006"}, {c | m, c, "-1003 +1000"}, {c | m, d, "-1003 -1000 +1002"}, {c | m, c | d, "-1003 +1002"},
		{c | m, m, "-1000 +1003"}, {c | m, c | m, ""}, {c | m, d | m, "-1000 +1002 +1003"}, {c | m, c | d | m, "+1002 +1003"},
		{d | m, 0, "-1003 -1002 -1006"}, {d | m, c, "-1003 -1002 +1000"}, {d | m, d, "-1003 +1002"}, {d | m, c | d, "-1003 +1000 +1002"},
		{d | m, m, "-1002 +1003"}, {d | m, c | m, "-1002 +1000 +1003"}, {d | m, d | m, ""}, {d | m, c | d | m, "+1000 +1003"},
		{c | d | m, 0, "-1003 -1002 -1000 -1006"}, {c | d | m, c, "-1003 -1002 +1000"}, {c | d | m, d, "-1003 -1000 +1002"}, {c | d | m, c | d, "-1003 +1002"},
		{c | d | m, m, "-1002 -1000 +1003"}, {c | d | m, c | m, "-1002 +1003"}, {c | d | m, d | m, "-1000 +1003"}, {c | d | m, c | d | m, ""},
	} {
		term := newTerm(&fakeBackend{}, ColorMode256)
		term.initialized = true // no writer runs, so what is sent stays queued
		term.SetMouseMode(tc.from)
		before := string(term.queued)
		term.queued = nil
		term.SetMouseMode(tc.to)
		got := string(term.queued)

		var want strings.Builder
		for _, f := range strings.Fields(tc.want) {
			want.WriteString("\x1b[?" + f[1:] + map[byte]string{'+': "h", '-': "l"}[f[0]])
		}
		if got != want.String() {
			t.Errorf("%03b to %03b: %q, want %q", tc.from, tc.to, got, want.String())
		}
		one, alacritty, each, sgr := reportedMouseMode(before + got)
		if top := highestMouseMode(tc.to); one != top || alacritty != top || each != top || sgr != (tc.to != 0) {
			t.Errorf("%03b to %03b reports by %03b, %03b, %03b, SGR %t; want %03b", tc.from, tc.to, one, alacritty, each, sgr, top)
		}
	}
}

// reportedMouseMode replays mode sets and resets as each kind of terminal
// keeps them and returns the mode each then reports by: xterm, xterm.js and
// kitty keep the last mode set, which a reset of any turns off; alacritty's set
// clears the others and its reset only its own; VTE, WezTerm and Windows
// Terminal keep each, reporting by the highest
func reportedMouseMode(out string) (one, alacritty, each MouseMode, sgr bool) {
	modes := map[string]MouseMode{"1000": MouseModeClick, "1002": MouseModeDrag, "1003": MouseModeMotion}
	for _, s := range strings.Split(out, "\x1b[?")[1:] {
		code, set := s[:len(s)-1], strings.HasSuffix(s, "h")
		switch m := modes[code]; {
		case code == "1006":
			sgr = set
		case set:
			one, alacritty, each = m, m, each|m
		default:
			one, alacritty, each = 0, alacritty&^m, each&^m
		}
	}
	return one, alacritty, highestMouseMode(each), sgr
}

func highestMouseMode(m MouseMode) MouseMode {
	for m&(m-1) != 0 {
		m &= m - 1
	}
	return m
}

// A terminal that stops reading holds the write in flight, not whoever draws:
// Flush returns while it is out, Blocked says how long it has waited, the newest
// frame held meanwhile is the one drawn once the terminal reads again, and a
// control sequence asked for after a held frame reaches the terminal after it
func TestAStoppedTerminalHoldsNoCaller(t *testing.T) {
	f := &fakeBackend{gate: make(chan struct{})}
	term := newTerm(f, ColorMode256)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first write to go out", func() bool { return term.Blocked() > 0 })

	frame := func(r rune) []Cell { return []Cell{{Rune: r}} }
	returned := make(chan struct{})
	go func() {
		term.Flush(frame('a'), 1, 1)
		term.Flush(frame('b'), 1, 1)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush waited on a terminal that stopped reading")
	}
	time.Sleep(10 * time.Millisecond)
	if b := term.Blocked(); b < 10*time.Millisecond {
		t.Fatalf("blocked %v while the write is held", b)
	}
	term.SetCursorVisible(true)

	close(f.gate)
	waitFor(t, "output to drain", func() bool { return term.Blocked() == 0 && strings.Contains(f.written(), "\x1b[?25h") })
	got := f.written()
	if strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("drew %q; want the newest held frame only", got)
	}
	if strings.Index(got, "b") > strings.Index(got, "\x1b[?25h") {
		t.Fatalf("the cursor was shown before the frame held ahead of it: %q", got)
	}
	term.Fini()
}

// Fini restores the termios even when the terminal never reads its last
// sequences: it stops waiting after finiDrainTimeout
func TestFiniLeavesAStoppedTerminal(t *testing.T) {
	f := &fakeBackend{gate: make(chan struct{})}
	defer close(f.gate)
	term := newTerm(f, ColorMode256)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { term.Fini(); close(done) }()
	select {
	case <-done:
	case <-time.After(finiDrainTimeout + 2*time.Second):
		t.Fatal("Fini waited on a terminal that stopped reading")
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// A Terminal built over a caller's backend draws a flushed frame to it
func TestNewWithBackendFlushesToTheBackend(t *testing.T) {
	f := &fakeBackend{}
	term := NewWithBackend(f, ColorMode256)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	if term.ColorMode() != ColorMode256 {
		t.Fatalf("color mode %v", term.ColorMode())
	}
	term.Flush([]Cell{{Rune: 'x'}}, 1, 1)
	waitFor(t, "the frame to reach the backend", func() bool { return strings.Contains(f.written(), "x") })
}
