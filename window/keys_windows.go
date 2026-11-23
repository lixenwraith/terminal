package window

import (
	"unicode/utf16"
	"unsafe"

	"github.com/lixenwraith/terminal"
)

// Virtual-key codes the window reads by name; a letter's or a digit's is its
// ASCII, the letter in upper case
const (
	vkShift, vkControl, vkMenu = 0x10, 0x11, 0x12
	vkTab, vkReturn, vkInsert  = 0x09, 0x0D, 0x2D
	vkNumpad0, vkAdd, vkSub    = 0x60, 0x6B, 0x6D
	vkF4, vkF11                = 0x73, 0x7A
	vkOEMPlus, vkOEMMinus      = 0xBB, 0xBD
	vkProcessKey               = 0xE5 // a key an input method has taken
)

// vkKeys maps the virtual-key codes of non-printing keys to the key a terminal
// reports for them; the keypad sends the same codes with Num Lock off
var vkKeys = [256]terminal.Key{
	0x1B: terminal.KeyEscape, vkReturn: terminal.KeyEnter, vkTab: terminal.KeyTab, 0x08: terminal.KeyBackspace,
	0x2E: terminal.KeyDelete, vkInsert: terminal.KeyInsert,
	0x26: terminal.KeyUp, 0x28: terminal.KeyDown, 0x25: terminal.KeyLeft, 0x27: terminal.KeyRight,
	0x24: terminal.KeyHome, 0x23: terminal.KeyEnd, 0x21: terminal.KeyPageUp, 0x22: terminal.KeyPageDown,
	0x70: terminal.KeyF1, 0x71: terminal.KeyF2, 0x72: terminal.KeyF3, vkF4: terminal.KeyF4,
	0x74: terminal.KeyF5, 0x75: terminal.KeyF6, 0x76: terminal.KeyF7, 0x77: terminal.KeyF8,
	0x78: terminal.KeyF9, 0x79: terminal.KeyF10, vkF11: terminal.KeyF11, 0x7B: terminal.KeyF12,
}

// noKey is native.down when no key message's characters are waited for
const noKey = ^uint32(0)

// held is the modifiers down as of the message being handled
func held() terminal.Modifier {
	var mods terminal.Modifier
	for _, m := range [...]struct {
		vk  uintptr
		mod terminal.Modifier
	}{{vkShift, terminal.ModShift}, {vkMenu, terminal.ModAlt}, {vkControl, terminal.ModCtrl}} {
		if state, _, _ := procGetKeyState.Call(m.vk); state&0x8000 != 0 {
			mods |= m.mod
		}
	}
	return mods
}

// keyCode names the physical key of a key message for its hold: its scan code with
// the extended bit, or the virtual key where an injected one has no scan code
func keyCode(vk, lParam uintptr) uint32 {
	if scan := uint32(lParam>>16) & 0x1ff; scan&0xff != 0 {
		return scan
	}
	return 0x200 | uint32(vk)
}

// zoomKey is the zoom step of a key going down with Ctrl: = or + in, - out and 0
// back to the font's own, the keypad's too, as a factor, 0 for the last. A key is
// one of them by its name or by r, its unshifted character: the name holds on a
// layout that shifts for a digit, the character where a key is named otherwise.
func zoomKey(vk uintptr, r rune) (by float64, ok bool) {
	switch {
	case vk == vkOEMPlus, vk == vkAdd, r == '=', r == '+':
		return zoomStep, true
	case vk == vkOEMMinus, vk == vkSub, r == '-':
		return 1 / zoomStep, true
	case vk == '0', vk == vkNumpad0, r == '0':
		return 0, true
	}
	return 0, false
}

// keyDown takes a key going down: a chord the window keeps for itself, a
// non-printing key or a Ctrl chord, posted at once, or a key whose press is the
// characters TranslateMessage queued behind it
func (w *Window) keyDown(vk, lParam uintptr) {
	mods, repeat, code := held(), lParam>>30&1 != 0, keyCode(vk, lParam)
	w.down, w.typed = uint32(lParam>>16)&0x1ff, false
	key := vkKeys[uint8(vk)]
	switch {
	case vk == vkF11 && mods == 0, vk == vkReturn && mods == terminal.ModAlt:
		if !repeat {
			w.fullscreen(!w.full)
		}
	case vk == 'V' && mods == terminal.ModCtrl|terminal.ModShift, vk == vkInsert && mods == terminal.ModShift:
		if !repeat {
			w.paste()
		}
	case key != terminal.KeyNone:
		if key == terminal.KeyTab && mods&terminal.ModShift != 0 {
			key = terminal.KeyBacktab
		}
		ev, _ := keyEvent(key, 0, mods)
		w.keys.Down(code, ev, repeat)
	// Ctrl makes a chord of the key's own character: a letter key's name, whatever
	// script its layout types, or else what the layout gives it unshifted. With Alt
	// it is AltGr where the layout types a character for the three, then queued.
	case mods&terminal.ModCtrl != 0 && (mods&terminal.ModAlt == 0 || !w.queued()):
		r := rune(vk)
		if vk < 'A' || vk > 'Z' {
			c, _, _ := procMapVirtualKeyW.Call(vk, mapvkVKToChar)
			if r = rune(uint32(c)); r < 0 { // the top bit marks a dead key
				r = 0
			}
		}
		if by, zooms := zoomKey(vk, r); zooms && mods&terminal.ModAlt == 0 {
			w.zoom(by)
		} else if ev, ok := keyEvent(terminal.KeyNone, r, mods); ok {
			w.keys.Down(code, ev, repeat)
		}
	case vk == vkProcessKey || w.down&0xff == 0:
		w.down = noKey // an input method's, or injected with no key: its text is of no key
	default:
		w.typed = true
	}
}

// queued reports whether TranslateMessage queued a character behind the key
// message being handled
func (w *Window) queued() bool {
	var m msg
	r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), w.hwnd, wmChar, wmDeadChar, pmNoRemove)
	return r != 0
}

// char takes a UTF-16 unit of text: the press of the key going down that queued
// it, or text of no key, an input method's or an Alt code's, which is let go as it
// is pressed. alt is a character typed with Alt held.
func (w *Window) char(alt bool, unit rune, lParam uintptr) {
	own := lParam>>31&1 == 0 && uint32(lParam>>16)&0x1ff == w.down
	if own && !w.typed {
		return // its key was posted as it went down
	}
	if w.high != 0 {
		unit, w.high = utf16.DecodeRune(w.high, unit), 0
	} else if utf16.IsSurrogate(unit) {
		w.high = unit // the pair's second half follows
		return
	}
	if unit < ' ' || unit == 0x7f {
		return // a control character is a key's, which was posted
	}
	var mods terminal.Modifier
	if alt {
		mods = terminal.ModAlt
	}
	ev, _ := keyEvent(terminal.KeyNone, unit, mods)
	if own {
		w.keys.Down(w.down, ev, lParam>>30&1 != 0)
	} else {
		w.keys.Tap(ev)
	}
}
