package terminal

import (
	"bytes"
	"cmp"
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
}

func (f *fakeBackend) Init() error                              { return nil }
func (f *fakeBackend) Fini()                                    {}
func (f *fakeBackend) Size() (int, int)                         { return 1, 1 }
func (f *fakeBackend) SetResizeHandler(func(width, height int)) {}

func (f *fakeBackend) Write(p []byte) error {
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
		f := &fakeBackend{}
		o := newOutputBuffer(f, c.mode)
		o.flush(c.cells, len(c.cells), 1)
		if got, want := f.written(), "\x1b[1;1H"+c.want+"\x1b[0m"; got != want {
			t.Errorf("mode %d %+v: %q, want %q", c.mode, c.cells, got, want)
		}
	}
}

// A cell is drawn as one glyph: a control character, C0 or C1, a format one
// (a bidi control, a zero-width space) or a line separator that a pasted
// value or a log line carried in, or no rune at all, is drawn as a space,
// never sent to the terminal
func TestCellsSendNoControlCharacters(t *testing.T) {
	f := &fakeBackend{}
	o := newOutputBuffer(f, ColorMode256)
	o.flush([]Cell{{Rune: 0x1b}, {Rune: 0x9b}, {Rune: 0x7f}, {Rune: -229}, {Rune: 0xd800}, {Rune: 0x202e}, {Rune: 0x2067}, {Rune: 0x061c}, {Rune: 0x200b}, {Rune: 0x2028}, {Rune: 'é'}}, 11, 1)
	if got, want := f.written(), "\x1b[1;1H\x1b[0;38;5;16;48;5;16m          é\x1b[0m"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
}

// A cell is one column: a rune a terminal draws in none (a combining mark, a
// joining jamo), in two (wide, fullwidth, emoji) or in its neighbour's cluster
// (a spacing mark, a letter such as Thai's sara am) is drawn as U+FFFD, any
// other as itself
func TestEachCellIsOneColumn(t *testing.T) {
	f := &fakeBackend{}
	o := newOutputBuffer(f, ColorMode256)
	var row []Cell
	for _, r := range "\u0301\u20dd\u1161漢Ａ😀✅\U0001f1fa\u093e\u0e33é─❤" {
		row = append(row, Cell{Rune: r})
	}
	o.flush(row, len(row), 1)
	if got, want := f.written(), "\x1b[1;1H\x1b[0;38;5;16;48;5;16m"+strings.Repeat("\ufffd", 10)+"é─❤\x1b[0m"; got != want {
		t.Fatalf("%q, want %q", got, want)
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
	term := &termImpl{backend: f, syntheticCh: make(chan Event, 1), resizeCh: make(chan ResizeEvent, 1)}
	term.output = newOutputBuffer(f, ColorMode256)
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
	term := &termImpl{backend: f, syntheticCh: make(chan Event, 1), resizeCh: make(chan ResizeEvent, 1)}
	term.output = newOutputBuffer(f, ColorModeTrueColor)
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
