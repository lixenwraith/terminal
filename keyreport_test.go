package terminal

import (
	"slices"
	"testing"
	"time"
)

// A release carries what its press carried, whatever the source read when the
// key came up; a report the holds contradict is taken as one the source lost
func TestKeyHoldsPairReleasesWithPresses(t *testing.T) {
	press := func(r rune) Event { return Event{Type: EventKey, Key: KeyRune, Rune: r} }
	as := func(ev Event, a KeyAction, lost bool) Event { ev.Action, ev.Lost = a, lost; return ev }
	var h KeyHolds
	var got []Event
	send := func(ev Event) { got = append(got, ev) }
	report := func(code uint32, ev Event, a KeyAction) { h.Report(code, as(ev, a, false), send) }

	report(30, press('A'), KeyPress)   // Shift+a
	report(30, press('A'), KeyRepeat)  // held
	report(30, press('a'), KeyRelease) // Shift came up first
	report(31, press('s'), KeyRelease) // never pressed
	report(32, press('d'), KeyRepeat)  // its press was lost
	report(32, press('d'), KeyPress)   // and so was its release
	report(33, press('f'), KeyPress)
	h.ReleaseAll(send) // focus left
	want := []Event{
		press('A'), as(press('A'), KeyRepeat, false), as(press('A'), KeyRelease, false),
		press('d'), as(press('d'), KeyRelease, true), press('d'),
		press('f'), as(press('d'), KeyRelease, true), as(press('f'), KeyRelease, true),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if h.Held(32) || h.Held(33) || len(h.held) != 0 {
		t.Fatalf("holds left after ReleaseAll: %+v", h.held)
	}
}

// A source that never releases can't hold more keys down than maxHolds: the
// oldest hold ends to make room
func TestKeyHoldsStayBounded(t *testing.T) {
	var h KeyHolds
	var lost []rune
	for code := range uint32(maxHolds + 2) {
		h.Report(code, Event{Type: EventKey, Key: KeyRune, Rune: 'a' + rune(code)}, func(ev Event) {
			if ev.Action == KeyRelease && ev.Lost {
				lost = append(lost, ev.Rune)
			}
		})
	}
	if len(h.held) != maxHolds || !slices.Equal(lost, []rune{'a', 'b'}) || !h.Held(maxHolds+1) {
		t.Fatalf("%d held, lost %q", len(h.held), lost)
	}
}

// Focus reports are read only while asked for; as focus leaves, the keys held
// are released first, marked Lost, since their own releases go elsewhere
func TestFocusEndsHolds(t *testing.T) {
	r := &inputReader{eventCh: make(chan Event, 16)}
	if r.parseInput([]byte("\x1b[O\x1b[Ix")); len(r.eventCh) != 1 || (<-r.eventCh) != key('x') {
		t.Fatal("focus reports read while not asked for")
	}

	r.focus = true
	w := Event{Type: EventKey, Key: KeyRune, Rune: 'w'}
	r.holds.Report(17, w, func(Event) {})
	r.parseInput([]byte("\x1b[O\x1b[I"))
	close(r.eventCh)
	var got []Event
	for ev := range r.eventCh {
		got = append(got, ev)
	}
	w.Action, w.Lost = KeyRelease, true
	if want := []Event{w, {Type: EventFocus}, {Type: EventFocus, Focused: true}}; !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A terminal with no source of releases answers every ask with presses and
// sends the terminal nothing for it
func TestKeyReportWithoutSourceIsPresses(t *testing.T) {
	f := &fakeBackend{}
	term := newTerm(f, ColorMode256)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	waitFor(t, "Init's output", func() bool { return f.written() != "" })
	before := f.written()

	if err := term.SetKeyReport(KeyReportReleases+1, KeySourceProtocol); err == nil {
		t.Fatal("an unknown report was taken")
	}
	for _, report := range []KeyReport{KeyReportReleases, KeyReportPresses} {
		if err := term.SetKeyReport(report, KeySourceConsole|KeySourceProtocol); err != nil {
			t.Fatal(err)
		}
		got := make(chan Event, 1)
		go func() { got <- term.PollEvent() }()
		select {
		case ev := <-got:
			if ev != (Event{Type: EventKeyReport, Report: KeyReportPresses}) {
				t.Fatalf("asked for %v: %+v", report, ev)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("asked for %v: no answer", report)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if got := f.written(); got != before {
		t.Fatalf("asks wrote %q", got[len(before):])
	}
}

// At presses a repeat is one more press and a release is dropped, as on any
// terminal; at releases a key the terminal names no physical key for is a tap,
// its release at once, and a named one is held until its own release
func TestKeysAtEachLevel(t *testing.T) {
	as := func(ev Event, a KeyAction) Event { ev.Action = a; return ev }
	for _, c := range []struct {
		report KeyReport
		want   []Event
	}{
		{KeyReportPresses, []Event{key('x'), key('w'), key('w')}},
		{KeyReportReleases, []Event{key('x'), as(key('x'), KeyRelease), key('w'), as(key('w'), KeyRepeat), as(key('w'), KeyRelease)}},
	} {
		r := &inputReader{eventCh: make(chan Event, 16), report: c.report}
		r.sendKey(0, key('x'))
		r.sendKey(17, key('w'))
		r.sendKey(17, as(key('w'), KeyRepeat))
		r.sendKey(17, as(key('w'), KeyRelease))
		close(r.eventCh)
		var got []Event
		for ev := range r.eventCh {
			got = append(got, ev)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.report, got, c.want)
		}
	}
}
