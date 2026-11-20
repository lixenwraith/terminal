package terminal

import (
	"hash/fnv"
	"unicode"
	"unicode/utf8"
)

// Modifier bits as a page reports them on a key or a pointer event
const (
	pageShift = 1 << 0
	pageAlt   = 1 << 1
	pageCtrl  = 1 << 2
	pageMeta  = 1 << 3
)

// pageKeys maps KeyboardEvent.key values of non-printing keys to the key a terminal
// reports for them
var pageKeys = map[string]Key{
	"Escape": KeyEscape, "Enter": KeyEnter, "Tab": KeyTab,
	"Backspace": KeyBackspace, "Delete": KeyDelete,
	"ArrowUp": KeyUp, "ArrowDown": KeyDown,
	"ArrowLeft": KeyLeft, "ArrowRight": KeyRight,
	"Home": KeyHome, "End": KeyEnd,
	"PageUp": KeyPageUp, "PageDown": KeyPageDown, "Insert": KeyInsert,
	"F1": KeyF1, "F2": KeyF2, "F3": KeyF3, "F4": KeyF4,
	"F5": KeyF5, "F6": KeyF6, "F7": KeyF7, "F8": KeyF8,
	"F9": KeyF9, "F10": KeyF10, "F11": KeyF11, "F12": KeyF12,
}

// pageKey is the event for a key press on a page: key as the browser resolved it with
// Shift applied, mods the modifier bits. ok is false for a key a terminal never sees:
// a modifier alone, a dead key, a Meta chord, or the "Unidentified" a soft keyboard
// sends ahead of its text. An Alt chord never arrives: the page leaves it to the
// browser, as it leaves the browser's other chords.
func pageKey(key string, mods uint8) (Event, bool) {
	ev := Event{Type: EventKey}
	if mods&pageMeta != 0 {
		return ev, false
	}
	if k, ok := pageKeys[key]; ok {
		ev.Key = k
		if k == KeyTab && mods&pageShift != 0 {
			ev.Key, ev.Modifiers = KeyBacktab, ModShift
			return ev, true
		}
		// A terminal reports modifiers on the editing, navigation and function keys only
		if k == KeyDelete || k >= KeyUp && k <= KeyF12 {
			ev.Modifiers = pageMods(mods)
		}
		return ev, true
	}
	r, size := utf8.DecodeRuneInString(key)
	if size == 0 || size != len(key) || r == utf8.RuneError {
		return ev, false
	}
	if mods&pageCtrl != 0 {
		k, ok := ControlKey(unicode.ToLower(r))
		if !ok {
			return ev, false
		}
		ev.Key = k
	} else {
		ev.Key, ev.Rune = KeyRune, r
	}
	return ev, true
}

// pageKeyCode names the physical key a KeyboardEvent.code names, which pairs a keyup
// with its keydown however the layout or the modifiers changed the key between them
func pageKeyCode(code string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(code))
	return h.Sum32()
}

// pageMouse is the event for a page's pointer at cell x, y: button and action in the
// package's own numbering, which the page uses, a move being the one action without
// a button, and touch for a finger's; ok is false outside it
func pageMouse(x, y, button, action int, mods uint8, touch bool) (Event, bool) {
	if button < int(MouseBtnNone) || button > int(MouseBtnForward) ||
		action < int(MouseActionPress) || action > int(MouseActionDrag) ||
		(button == int(MouseBtnNone)) != (action == int(MouseActionMove)) {
		return Event{}, false
	}
	return Event{Type: EventMouse, MouseX: max(x, 0), MouseY: max(y, 0),
		MouseBtn: MouseButton(button), MouseAction: MouseAction(action),
		Modifiers: pageMods(mods), Touch: touch}, true
}

func pageMods(mods uint8) Modifier {
	var m Modifier
	if mods&pageShift != 0 {
		m |= ModShift
	}
	if mods&pageAlt != 0 {
		m |= ModAlt
	}
	if mods&pageCtrl != 0 {
		m |= ModCtrl
	}
	return m
}
