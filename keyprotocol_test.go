package terminal

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// decoded is what a reader decoding the key protocol sends for input at the
// report level given
func decoded(report KeyReport, in string) []Event {
	r := &inputReader{eventCh: make(chan Event, 256), decoding: true, report: report}
	r.parseInput([]byte(in))
	close(r.eventCh)
	var events []Event
	for ev := range r.eventCh {
		events = append(events, ev)
	}
	return events
}

// Every press the protocol reports reads as the key a legacy terminal sends for
// it reads, Alacritty's encoding of each the reference, so a game sees the
// same keys with the protocol on
func TestProtocolPressesReadAsLegacyKeys(t *testing.T) {
	for _, c := range []struct{ name, legacy, report string }{
		{"a", "a", "\x1b[97;1;97u"},
		{"shift+a", "A", "\x1b[97:65;2;65u"},
		{"ctrl+a", "\x01", "\x1b[97;5u"},
		{"alt+a", "\x1ba", "\x1b[97;3;97u"},
		{"ctrl+alt+a", "\x1b\x01", "\x1b[97;7u"},
		{"ctrl+shift+a", "\x01", "\x1b[97:65;6u"},
		{"alt+shift+a", "\x1bA", "\x1b[97:65;4;65u"},
		{"super+a", "a", "\x1b[97;9;97u"},
		{"caps lock a", "A", "\x1b[97;65;65u"},
		{"shift+1", "!", "\x1b[49:33;2;33u"},
		{"é", "é", "\x1b[233;1;233u"},
		{"shift+é", "É", "\x1b[233:201;2;201u"},
		{"altgr+q", "@", "\x1b[64;1;64u"},
		{"ctrl+[", "\x1b", "\x1b[91;5u"},
		{"ctrl+h", "\x08", "\x1b[104;5u"},
		{"ctrl+shift+2", "\x00", "\x1b[50:64;6u"},
		{"ctrl+/", "\x1f", "\x1b[47;5u"},
		{"space", " ", "\x1b[32;1;32u"},
		{"ctrl+space", "\x00", "\x1b[32;5u"},
		{"alt+space", "\x1b ", "\x1b[32;3;32u"},
		{"enter", "\r", "\x1b[13u"},
		{"shift+enter", "\r", "\x1b[13;2u"},
		{"alt+enter", "\x1b\r", "\x1b[13;3u"},
		{"tab", "\t", "\x1b[9u"},
		{"ctrl+tab", "\t", "\x1b[9;5u"},
		{"shift+tab", "\x1b[Z", "\x1b[9;2u"},
		{"alt+shift+tab", "\x1b\x1b[Z", "\x1b[9;4u"},
		{"backspace", "\x7f", "\x1b[127u"},
		{"ctrl+backspace", "\x08", "\x1b[127;5u"},
		{"alt+backspace", "\x1b\x7f", "\x1b[127;3u"},
		{"escape", "\x1b", "\x1b[27u"},
		{"shift+escape", "\x1b", "\x1b[27;2u"},
		{"alt+escape", "\x1b\x1b", "\x1b[27;3u"},
		{"up", "\x1b[A", "\x1b[A"},
		{"shift+up", "\x1b[1;2A", "\x1b[1;2A"},
		{"ctrl+alt+left", "\x1b[1;7D", "\x1b[1;7D"},
		{"home", "\x1b[H", "\x1b[H"},
		{"end", "\x1b[F", "\x1b[F"},
		{"f1", "\x1bOP", "\x1b[P"},
		{"ctrl+f1", "\x1b[1;5P", "\x1b[1;5P"},
		{"f3", "\x1bOR", "\x1b[13~"},
		{"f4", "\x1bOS", "\x1b[S"},
		{"f5", "\x1b[15~", "\x1b[15~"},
		{"shift+f12", "\x1b[24;2~", "\x1b[24;2~"},
		{"insert", "\x1b[2~", "\x1b[2~"},
		{"delete", "\x1b[3~", "\x1b[3~"},
		{"ctrl+delete", "\x1b[3;5~", "\x1b[3;5~"},
		{"page up", "\x1b[5~", "\x1b[5~"},
		{"page down", "\x1b[6~", "\x1b[6~"},
		{"keypad 1", "1", "\x1b[57400;1;49u"},
		{"keypad enter", "\r", "\x1b[57414u"},
		{"keypad left", "\x1b[D", "\x1b[57417u"},
		{"keypad shift+home", "\x1b[1;2H", "\x1b[57423;2u"},
		{"left shift alone", "", "\x1b[57441;2u"},
		{"caps lock alone", "", "\x1b[57358u"},
		{"f13", "", "\x1b[57376u"},
		{"volume", "", "\x1b[57439u"},
		{"keypad begin", "", "\x1b[E\x1b[57427~"},
		{"compose ü", "ü", "\x1b[0;;252u"},
	} {
		// An ESC or two alone read once input pauses
		want, _ := parsed(c.legacy)
		if ev, ok := loneEscape[c.legacy]; ok {
			want = []Event{ev}
		}
		if got := decoded(KeyReportPresses, c.report); !slices.Equal(got, want) {
			t.Errorf("%s: %q reads as %+v, want %+v as %q reads", c.name, c.report, got, want, c.legacy)
		}
	}

	// Ctrl with a character no control key has keeps Ctrl, which a legacy
	// terminal can't send
	if got, want := decoded(KeyReportPresses, "\x1b[49;5;49u"), (Event{Type: EventKey, Key: KeyRune, Rune: '1', Modifiers: ModCtrl}); len(got) != 1 || got[0] != want {
		t.Errorf("ctrl+1 reads as %+v, want %+v", got, want)
	}
}

// At releases each key reports its press, repeats and release, paired by the
// key: a release carries its press's key whatever was let go first; modifier
// keys alone report nothing; text with no key behind it is a tap
func TestProtocolReportsHolds(t *testing.T) {
	ev := func(k Key, r rune, m Modifier, a KeyAction) Event {
		return Event{Type: EventKey, Key: k, Rune: r, Modifiers: m, Action: a}
	}
	got := decoded(KeyReportReleases, strings.Join([]string{
		"\x1b[57441;2u", "\x1b[97:65;2;65u", // Shift down, a down: 'A'
		"\x1b[57441;1:3u", "\x1b[97;1:2;97u", // Shift up, a repeats as 'a'
		"\x1b[97;1:3u",                         // a up: 'A' comes up
		"\x1b[A", "\x1b[1;1:2A", "\x1b[1;1:3A", // Up down, repeat, up
		"\x1b[13u", "\x1b[13;1:3u", // Enter
		"\x1b[0;;252u",             // composed text
		"\x1b[98;1:3u",             // a release with no press
		"\x1b[3;5~", "\x1b[3;1:3~", // Ctrl+Delete down, Delete up after Ctrl
	}, ""))
	want := []Event{
		ev(KeyRune, 'A', 0, KeyPress), ev(KeyRune, 'a', 0, KeyRepeat), ev(KeyRune, 'A', 0, KeyRelease),
		ev(KeyUp, 0, 0, KeyPress), ev(KeyUp, 0, 0, KeyRepeat), ev(KeyUp, 0, 0, KeyRelease),
		ev(KeyEnter, 0, 0, KeyPress), ev(KeyEnter, 0, 0, KeyRelease),
		ev(KeyRune, 'ü', 0, KeyPress), ev(KeyRune, 'ü', 0, KeyRelease),
		ev(KeyDelete, 0, ModCtrl, KeyPress), ev(KeyDelete, 0, ModCtrl, KeyRelease),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

// fakeKeyTerm is a terminal answering the key protocol's requests as Alacritty
// does: CSI ? u with the flags in force, which a push and a pop change, and
// DA1. Without the protocol it answers DA1 alone; silent, nothing. grant is
// the flags it takes of a push, all when zero.
type fakeKeyTerm struct {
	fakeBackend
	has, silent bool
	grant       int
	stack       []int
}

var keyRequest = regexp.MustCompile(`\x1b\[(\?u|>(\d*)u|<u|c)`)

func (f *fakeKeyTerm) keySources() KeySource { return KeySourceProtocol }

func (f *fakeKeyTerm) Write(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out.Write(p)
	for _, m := range keyRequest.FindAllSubmatch(p, -1) {
		switch req := string(m[1]); {
		case f.silent:
		case req == "c":
			f.chunks = append(f.chunks, "\x1b[?62;22c")
		case !f.has:
		case req == "?u":
			flags := 0
			if len(f.stack) > 0 {
				flags = f.stack[len(f.stack)-1]
			}
			f.chunks = append(f.chunks, "\x1b[?"+strconv.Itoa(flags)+"u")
		case req == "<u":
			if len(f.stack) > 0 {
				f.stack = f.stack[:len(f.stack)-1]
			}
		default:
			flags, _ := strconv.Atoi(string(m[2]))
			if f.grant != 0 {
				flags &= f.grant
			}
			f.stack = append(f.stack, flags)
		}
	}
	return nil
}

// typed hands the reader input as though the terminal sent it
func (f *fakeKeyTerm) typed(in string) {
	f.mu.Lock()
	f.chunks = append(f.chunks, in)
	f.mu.Unlock()
}

// pushed is how many pushes the terminal holds
func (f *fakeKeyTerm) pushed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stack)
}

// poll is the terminal's next event, failing after two seconds
func poll(t *testing.T, term Terminal) Event {
	t.Helper()
	got := make(chan Event, 1)
	go func() { got <- term.PollEvent() }()
	select {
	case ev := <-got:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no event")
		return Event{}
	}
}

// A terminal with the protocol is asked about it, pushed the flags and asked
// what they came to before releases are reported; focus is reported with them,
// and they end, Lost, as focus leaves. Going back to presses releases what is
// held, pops the flags and stops focus reports, and Fini pops them too.
func TestProtocolSourceComesAndGoes(t *testing.T) {
	f := &fakeKeyTerm{has: true}
	term := newTerm(f, ColorMode256)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	term.SetKeyReport(KeyReportReleases, KeySourceConsole|KeySourceProtocol)
	if ev := poll(t, term); ev != (Event{Type: EventKeyReport, Report: KeyReportReleases, Source: KeySourceProtocol}) {
		t.Fatalf("asked for releases: %+v", ev)
	}
	waitFor(t, "focus reports", func() bool { return strings.Contains(f.written(), "\x1b[?1004h") })
	out := f.written()
	if i := strings.Index(out, "\x1b[?u\x1b[c\x1b[>31u\x1b[?u\x1b[c\x1b[?1004h"); i < 0 {
		t.Fatalf("requests %q", out)
	}

	w := Event{Type: EventKey, Key: KeyRune, Rune: 'w'}
	f.typed("\x1b[119;1;119u\x1b[O\x1b[I\x1b[119;1;119u")
	lost := w
	lost.Action, lost.Lost = KeyRelease, true
	for _, want := range []Event{w, lost, {Type: EventFocus}, {Type: EventFocus, Focused: true}, w} {
		if ev := poll(t, term); ev != want {
			t.Fatalf("got %+v, want %+v", ev, want)
		}
	}

	term.SetKeyReport(KeyReportPresses, 0)
	for _, want := range []Event{lost, {Type: EventKeyReport, Report: KeyReportPresses}} {
		if ev := poll(t, term); ev != want {
			t.Fatalf("back to presses: %+v, want %+v", ev, want)
		}
	}
	waitFor(t, "the pop", func() bool { return strings.Contains(f.written(), "\x1b[<u\x1b[?1004l\x1b[c") })
	if n := f.pushed(); n != 0 {
		t.Fatalf("%d pushes left", n)
	}

	// Asked again, the protocol is pushed without asking whether the terminal
	// has it, and Fini pops it before the screen it was pushed on goes
	term.SetKeyReport(KeyReportReleases, KeySourceProtocol)
	if ev := poll(t, term); ev.Source != KeySourceProtocol {
		t.Fatalf("asked again: %+v", ev)
	}
	term.Fini()
	out = f.written()
	if n := strings.Count(out, "\x1b[?u"); n != 3 {
		t.Fatalf("asked %d times, want a probe and two pushes: %q", n, out)
	}
	if i := strings.LastIndex(out, "\x1b[<u\x1b[?1004l"); i < 0 || i > strings.LastIndex(out, "\x1b[?1049l") {
		t.Fatalf("Fini left the flags pushed: %q", out)
	}
}

// A terminal without the protocol, one that takes the push without the flags
// releases need, and one that answers nothing all come to presses; the first
// two are asked once only
func TestProtocolMissingIsPresses(t *testing.T) {
	defer func(d time.Duration) { protoTimeout = d }(protoTimeout)
	protoTimeout = 100 * time.Millisecond
	for _, c := range []struct {
		name   string
		term   *fakeKeyTerm
		probes int
	}{
		{"legacy", &fakeKeyTerm{}, 1},
		{"partial", &fakeKeyTerm{has: true, grant: protoDisambiguate | protoEventTypes}, 2},
		{"silent", &fakeKeyTerm{silent: true}, 1},
	} {
		term := newTerm(c.term, ColorMode256)
		if err := term.Init(); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			term.SetKeyReport(KeyReportReleases, KeySourceProtocol)
			if ev := poll(t, term); ev != (Event{Type: EventKeyReport, Report: KeyReportPresses}) {
				t.Fatalf("%s: %+v", c.name, ev)
			}
		}
		term.Fini()
		out := c.term.written()
		if n := strings.Count(out, "\x1b[?u"); n != c.probes {
			t.Errorf("%s: asked %d times, want %d: %q", c.name, n, c.probes, out)
		}
		if strings.Count(out, "\x1b[>") != strings.Count(out, "\x1b[<u") || c.term.pushed() != 0 {
			t.Errorf("%s: flags left pushed: %q", c.name, out)
		}
	}
}

// decodedIn is what a reader decoding the protocol at releases sends for input
// arriving in chunks, and how many bytes it holds back
func decodedIn(chunks ...string) (events []Event, held int) {
	r := &inputReader{eventCh: make(chan Event, 4096), decoding: true, report: KeyReportReleases}
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

// Whatever a terminal reporting by the protocol sends, however reads split it,
// the reader sends the same events, holds back no more than a sequence, and
// never sends a release before a press of the key it carries
func FuzzProtocolNeverStalls(f *testing.F) {
	for _, s := range []string{"\x1b[97:65;2;65u\x1b[97;1:3u", "\x1b[A\x1b[1;1:3A", "\x1b[0;;252:253u", "\x1b[57441;2u\x1b[3;5:2~", "\x1b[O\x1b[13;1:3u", "\x1b[99;5:3u"} {
		f.Add(s, uint(len(s)/2))
	}
	f.Fuzz(func(t *testing.T, in string, cut uint) {
		cut %= uint(len(in)) + 1
		whole, held := decodedIn(in)
		if split, _ := decodedIn(in[:cut], in[cut:]); !slices.Equal(whole, split) {
			t.Fatalf("%q: whole %+v, split at %d %+v", in, whole, cut, split)
		}
		if held > csiMaxLen+1 {
			t.Fatalf("%q: holds %d bytes", in, held)
		}
		if _, held := decodedIn(in, "\x7f\x7f\x7f\x7f\x1b[201~"); held != 0 {
			t.Fatalf("%q: stalled holding %d bytes", in, held)
		}
		type id struct {
			k Key
			r rune
			m Modifier
		}
		down := map[id]int{}
		for _, ev := range whole {
			if ev.Type != EventKey {
				continue
			}
			k := id{ev.Key, ev.Rune, ev.Modifiers}
			switch ev.Action {
			case KeyPress:
				down[k]++
			case KeyRelease:
				if down[k]--; down[k] < 0 {
					t.Fatalf("%q: %+v released before its press", in, ev)
				}
			}
		}
	})
}
