package android

import (
	"unicode"

	"github.com/lixenwraith/terminal"
)

// Android's KeyEvent meta state bits
const (
	metaShift = 0x1
	metaAlt   = 0x2
	metaCtrl  = 0x1000
)

// androidKeys maps Android key codes to the keys a terminal reports for them; Back
// is Escape, the key that leaves what is open
var androidKeys = map[int32]terminal.Key{
	4:   terminal.KeyEscape, // KEYCODE_BACK
	111: terminal.KeyEscape,
	66:  terminal.KeyEnter,
	160: terminal.KeyEnter, // numeric keypad
	61:  terminal.KeyTab,
	67:  terminal.KeyBackspace, // KEYCODE_DEL
	112: terminal.KeyDelete,    // KEYCODE_FORWARD_DEL
	19:  terminal.KeyUp,
	20:  terminal.KeyDown,
	21:  terminal.KeyLeft,
	22:  terminal.KeyRight,
	122: terminal.KeyHome,
	123: terminal.KeyEnd,
	92:  terminal.KeyPageUp,
	93:  terminal.KeyPageDown,
	124: terminal.KeyInsert,
	131: terminal.KeyF1, 132: terminal.KeyF2, 133: terminal.KeyF3, 134: terminal.KeyF4,
	135: terminal.KeyF5, 136: terminal.KeyF6, 137: terminal.KeyF7, 138: terminal.KeyF8,
	139: terminal.KeyF9, 140: terminal.KeyF10, 141: terminal.KeyF11, 142: terminal.KeyF12,
}

// keyEvent is the terminal event for an Android key: a named key, Ctrl with a
// character as the control byte a terminal reads, or the character it types. As
// a terminal, it reports modifiers on the editing, navigation and function keys
// only, and Alt on a character.
func keyEvent(code int32, char rune, meta int32) (terminal.Event, bool) {
	ev := terminal.Event{Type: terminal.EventKey}
	if key, ok := androidKeys[code]; ok {
		ev.Key = key
		if key == terminal.KeyTab && meta&metaShift != 0 {
			ev.Key, ev.Modifiers = terminal.KeyBacktab, terminal.ModShift
		}
		if key == terminal.KeyDelete || key >= terminal.KeyUp && key <= terminal.KeyF12 {
			ev.Modifiers = modifiers(meta)
		}
		return ev, true
	}
	if char == 0 {
		return ev, false
	}
	if meta&metaCtrl != 0 {
		lower := unicode.ToLower(char)
		k, ok := terminal.ControlKey(lower)
		if !ok {
			return ev, false
		}
		ev.Key = k
	} else {
		ev.Key, ev.Rune = terminal.KeyRune, char
	}
	ev.Modifiers = modifiers(meta) & terminal.ModAlt
	return ev, true
}

func modifiers(meta int32) terminal.Modifier {
	var m terminal.Modifier
	if meta&metaShift != 0 {
		m |= terminal.ModShift
	}
	if meta&metaAlt != 0 {
		m |= terminal.ModAlt
	}
	if meta&metaCtrl != 0 {
		m |= terminal.ModCtrl
	}
	return m
}

// KeyDown is a key the view received going down, or repeating while held:
// Android's key code, the character it types without Ctrl or Alt (0 for none) and
// its meta state. It reports whether a terminal has the key; one it has not, a
// volume key, is left to the system.
func (v *View) KeyDown(code, typed, meta int32, repeat bool) bool {
	ev, ok := keyEvent(code, rune(typed), meta)
	if ok {
		v.keys.Down(uint32(code), ev, repeat)
	}
	return ok
}

// KeyUp is a key the view received coming up, taken as its KeyDown was
func (v *View) KeyUp(code, typed, meta int32) bool {
	_, ok := keyEvent(code, rune(typed), meta)
	if ok {
		v.keys.Up(uint32(code))
	}
	return ok
}

// Key is a key pressed and let go at once, with no up of its own to come: Back, or
// the system keyboard's delete
func (v *View) Key(code, typed, meta int32) bool {
	ev, ok := keyEvent(code, rune(typed), meta)
	if ok {
		v.keys.Tap(ev)
	}
	return ok
}

// Text is what the system keyboard committed or a paste brought: one event carries
// it, so a long paste fits the queue whole
func (v *View) Text(s string) {
	if s != "" {
		v.Post(terminal.Event{Type: terminal.EventPaste, Text: s})
	}
}

// FocusLost ends every key held: the window's keys go elsewhere, their ups too
func (v *View) FocusLost() { v.keys.Lost() }

// Touch is a finger going down or up at the view's pixel x, y: the left button
// pressed or released on the cell under it
func (v *View) Touch(x, y int, down bool) {
	if ev, ok := v.touch(x, y, down); ok {
		v.Post(ev)
	}
}

// Drag is a pressed finger moving to the view's pixel x, y: the left button dragged
// to the cell under it, once per cell
func (v *View) Drag(x, y int) {
	if ev, ok := v.drag(x, y); ok {
		v.Post(ev)
	}
}

// OnBar reports whether the view's pixel row y is drawn on the bar, where a finger
// presses with the pointer off
func (v *View) OnBar(y int) bool {
	_, cy := v.cellAt(0, y)
	return v.onBar(cy)
}

// touch is the left button pressed or released by a finger on the cell drawn under
// pixel x, y, the nearest edge cell for one past the view's edge. With the pointer
// off the bar, the last rows, still takes a press, where a program can offer the
// pointer back; a release follows a press that was taken.
func (v *View) touch(x, y int, down bool) (terminal.Event, bool) {
	cx, cy := v.cellAt(x, y)
	if !v.pointer.Load() && (down && !v.onBar(cy) || !down && !v.finger.down) {
		return terminal.Event{}, false
	}
	v.finger.down, v.finger.x, v.finger.y = down, cx, cy
	action := terminal.MouseActionRelease
	if down {
		action = terminal.MouseActionPress
	}
	return terminal.Event{Type: terminal.EventMouse, MouseBtn: terminal.MouseBtnLeft, MouseAction: action,
		MouseX: cx, MouseY: cy, Touch: true}, true
}

// drag is a pressed finger moving: the left button dragged to the cell drawn under
// pixel x, y, reported once per cell as a terminal reports motion
func (v *View) drag(x, y int) (terminal.Event, bool) {
	if !v.pointer.Load() || !v.finger.down {
		return terminal.Event{}, false
	}
	cx, cy := v.cellAt(x, y)
	if cx == v.finger.x && cy == v.finger.y {
		return terminal.Event{}, false
	}
	v.finger.x, v.finger.y = cx, cy
	return terminal.Event{Type: terminal.EventMouse, MouseBtn: terminal.MouseBtnLeft,
		MouseAction: terminal.MouseActionDrag, MouseX: cx, MouseY: cy, Touch: true}, true
}

// onBar reports whether row cy is the bar's, one of the grid's last rows
func (v *View) onBar(cy int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return cy >= v.rows-v.barRows
}

// cellAt is the cell drawn under pixel x, y, the grid stretched over the view; the
// nearest edge cell for one past its edge
func (v *View) cellAt(x, y int) (int, int) {
	v.mu.Lock()
	cols, rows, w, h := v.cols, v.rows, v.w, v.h
	v.mu.Unlock()
	return min(max(x*cols/w, 0), cols-1), min(max(y*rows/h, 0), rows-1)
}
