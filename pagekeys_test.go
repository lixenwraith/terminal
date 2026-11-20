package terminal

import "testing"

// TestAPagesKeyPressReportsWhatATerminalReports: a KeyboardEvent's key and modifiers
// become the event parsed from the bytes a terminal sends for that key; what a
// terminal never sees is refused.
func TestAPagesKeyPressReportsWhatATerminalReports(t *testing.T) {
	const shift, alt, ctrl, meta = pageShift, pageAlt, pageCtrl, pageMeta
	key := func(k Key, m Modifier) Event {
		return Event{Type: EventKey, Key: k, Modifiers: m}
	}
	char := func(r rune, m Modifier) Event {
		return Event{Type: EventKey, Key: KeyRune, Rune: r, Modifiers: m}
	}
	for _, c := range []struct {
		key  string
		mods uint8
		want Event
	}{
		{"a", 0, char('a', 0)},
		{"A", shift, char('A', 0)},
		{" ", 0, char(' ', 0)},
		{"λ", 0, char('λ', 0)},
		{"c", ctrl, key(KeyCtrlC, 0)},
		{"C", ctrl | shift, key(KeyCtrlC, 0)},
		{"h", ctrl, key(KeyBackspace, 0)},
		{"[", ctrl, key(KeyEscape, 0)},
		{" ", ctrl, key(KeyCtrlSpace, 0)},
		{"Enter", 0, key(KeyEnter, 0)},
		{"Enter", ctrl, key(KeyEnter, 0)},
		{"Tab", 0, key(KeyTab, 0)},
		{"Tab", shift, key(KeyBacktab, ModShift)},
		{"Backspace", 0, key(KeyBackspace, 0)},
		{"Delete", ctrl, key(KeyDelete, ModCtrl)},
		{"ArrowUp", shift, key(KeyUp, ModShift)},
	} {
		if got, ok := pageKey(c.key, c.mods); !ok || got != c.want {
			t.Errorf("%q with %03b: %+v %v, want %+v", c.key, c.mods, got, ok, c.want)
		}
	}
	for _, c := range []struct {
		key  string
		mods uint8
	}{
		{"Shift", shift}, {"Dead", 0}, {"Unidentified", 0}, {"Process", 0},
		{"v", meta}, {"ArrowUp", meta}, {"1", ctrl}, {"", 0},
	} {
		if ev, ok := pageKey(c.key, c.mods); ok {
			t.Errorf("%q with %03b was taken as %+v", c.key, c.mods, ev)
		}
	}
}

// TestAPagesPointerIsReportedInTheTerminalsNumbering: button and action as the page
// numbers them are the terminal's, with the modifiers and whether a finger made them,
// and a move is the one action without a button; a number outside either is refused.
func TestAPagesPointerIsReportedInTheTerminalsNumbering(t *testing.T) {
	want := Event{Type: EventMouse, MouseX: 3, MouseY: 7,
		MouseBtn: MouseBtnRight, MouseAction: MouseActionPress, Modifiers: ModCtrl}
	if got, ok := pageMouse(3, 7, 3, 1, pageCtrl, false); !ok || got != want {
		t.Errorf("right press: %+v %v, want %+v", got, ok, want)
	}
	want = Event{Type: EventMouse, MouseX: 0, MouseY: 0,
		MouseBtn: MouseBtnWheelDown, MouseAction: MouseActionPress}
	if got, ok := pageMouse(-1, -2, 5, 1, 0, false); !ok || got != want {
		t.Errorf("wheel: %+v %v, want %+v", got, ok, want)
	}
	want = Event{Type: EventMouse, MouseX: 2, MouseY: 5,
		MouseBtn: MouseBtnLeft, MouseAction: MouseActionDrag, Touch: true}
	if got, ok := pageMouse(2, 5, 1, 4, 0, true); !ok || got != want {
		t.Errorf("finger: %+v %v, want %+v", got, ok, want)
	}
	want = Event{Type: EventMouse, MouseX: 4, MouseY: 1, MouseAction: MouseActionMove}
	if got, ok := pageMouse(4, 1, 0, 3, 0, false); !ok || got != want {
		t.Errorf("move: %+v %v, want %+v", got, ok, want)
	}
	for _, c := range [][2]int{{0, 1}, {0, 4}, {1, 3}, {8, 1}, {1, 0}, {1, 5}} {
		if ev, ok := pageMouse(0, 0, c[0], c[1], 0, false); ok {
			t.Errorf("button %d action %d was taken as %+v", c[0], c[1], ev)
		}
	}
}
