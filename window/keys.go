//go:build linux || freebsd

package window

import "github.com/lixenwraith/terminal"

// X modifier state bits
const (
	stateShift   = 1 << 0
	stateControl = 1 << 2
	stateMod1    = 1 << 3 // Alt on every common layout
	stateButton1 = 1 << 8
	stateButton2 = 1 << 9
	stateButton3 = 1 << 10
)

// keysymKeys maps the X keysyms of non-printing keys to the key a terminal reports
// for them, keypad variants included
var keysymKeys = map[uint64]terminal.Key{
	0xff1b: terminal.KeyEscape, 0xff0d: terminal.KeyEnter, 0xff8d: terminal.KeyEnter,
	0xff09: terminal.KeyTab, 0xfe20: terminal.KeyBacktab, 0xff08: terminal.KeyBackspace,
	0xffff: terminal.KeyDelete, 0xff9f: terminal.KeyDelete,
	0xff52: terminal.KeyUp, 0xff97: terminal.KeyUp, 0xff54: terminal.KeyDown, 0xff99: terminal.KeyDown,
	0xff51: terminal.KeyLeft, 0xff96: terminal.KeyLeft, 0xff53: terminal.KeyRight, 0xff98: terminal.KeyRight,
	0xff50: terminal.KeyHome, 0xff95: terminal.KeyHome, 0xff57: terminal.KeyEnd, 0xff9c: terminal.KeyEnd,
	0xff55: terminal.KeyPageUp, 0xff9a: terminal.KeyPageUp, 0xff56: terminal.KeyPageDown, 0xff9b: terminal.KeyPageDown,
	0xff63: terminal.KeyInsert, 0xff9e: terminal.KeyInsert,
	0xffbe: terminal.KeyF1, 0xffbf: terminal.KeyF2, 0xffc0: terminal.KeyF3, 0xffc1: terminal.KeyF4,
	0xffc2: terminal.KeyF5, 0xffc3: terminal.KeyF6, 0xffc4: terminal.KeyF7, 0xffc5: terminal.KeyF8,
	0xffc6: terminal.KeyF9, 0xffc7: terminal.KeyF10, 0xffc8: terminal.KeyF11, 0xffc9: terminal.KeyF12,
}

// keyEvent is the terminal event for a key press: keysym as the layout resolved it
// with Shift applied, state the modifier bits. ok is false for a key a terminal
// never sees, a modifier alone or a dead key.
func keyEvent(keysym uint64, state uint32) (terminal.Event, bool) {
	ev := terminal.Event{Type: terminal.EventKey}
	if k, ok := keysymKeys[keysym]; ok {
		ev.Key = k
		if k == terminal.KeyBacktab {
			ev.Modifiers = terminal.ModShift
		}
		// A terminal reports modifiers on the editing, navigation and function keys only
		if k == terminal.KeyDelete || k >= terminal.KeyUp && k <= terminal.KeyF12 {
			ev.Modifiers = modifiers(state)
		}
		return ev, true
	}
	r := keysymRune(keysym)
	if r == 0 {
		return ev, false
	}
	if state&stateControl != 0 {
		lower := r
		if lower >= 'A' && lower <= 'Z' {
			lower += 'a' - 'A'
		}
		k, ok := terminal.ControlKey(lower)
		if !ok {
			return ev, false
		}
		ev.Key = k
		if state&stateMod1 != 0 {
			ev.Modifiers = terminal.ModAlt
		}
		return ev, true
	}
	ev.Key, ev.Rune = terminal.KeyRune, r
	if state&stateMod1 != 0 {
		ev.Modifiers = terminal.ModAlt
	}
	return ev, true
}

// zoomStep is the factor one press or wheel step zooms the cell by, as on a page
const zoomStep = 1.1

// zoomKey is the zoom step of a key press: Ctrl with = or + in, with - out and with
// 0 back to the font's own, the keypad's too, as a factor, 0 for the last. A
// terminal emulator and a browser keep these chords for their own zoom, and no
// terminal reports them, so no game key is taken.
func zoomKey(keysym uint64, state uint32) (by float64, ok bool) {
	if state&stateControl == 0 || state&stateMod1 != 0 {
		return 0, false
	}
	switch keysym {
	case '=', '+', 0xffab:
		return zoomStep, true
	case '-', 0xffad:
		return 1 / zoomStep, true
	case '0', 0xffb0:
		return 0, true
	}
	return 0, false
}

// zoomWheel is the zoom step of a wheel step with Ctrl held, as on a page, whose
// pointer mode it takes no part in
func zoomWheel(button int32, state uint32) (by float64, ok bool) {
	switch {
	case state&stateControl == 0:
	case button == 4:
		return zoomStep, true
	case button == 5:
		return 1 / zoomStep, true
	}
	return 0, false
}

// keysymRune is the character of a printing keysym: Latin-1 keysyms are their
// code points and the Unicode block is offset by 0x01000000
func keysymRune(keysym uint64) rune {
	switch {
	case keysym >= 0x20 && keysym <= 0x7e, keysym >= 0xa0 && keysym <= 0xff:
		return rune(keysym)
	case keysym >= 0x01000100 && keysym <= 0x0110ffff:
		return rune(keysym - 0x01000000)
	}
	return 0
}

func modifiers(state uint32) terminal.Modifier {
	var m terminal.Modifier
	if state&stateShift != 0 {
		m |= terminal.ModShift
	}
	if state&stateMod1 != 0 {
		m |= terminal.ModAlt
	}
	if state&stateControl != 0 {
		m |= terminal.ModCtrl
	}
	return m
}
