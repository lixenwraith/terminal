package terminal

import (
	"strconv"
	"unicode/utf8"
)

// The Linux console's keymap and its translation of keycodes, after the
// kernel's own (drivers/tty/vt/keyboard.c), so a keyboard switched to raw
// keycodes types what the console types with it cooked. Keysyms are kept as
// the kernel keeps them: 0xf000 | type<<8 | value for a typed one, anything
// less a Unicode code point.

// Keysym types (linux/keyboard.h)
const (
	ktLatin  = iota // a Latin-1 character
	ktFn            // a function key's string
	ktSpec          // a console function: Enter, the locks, console switching
	ktPad           // a keypad key
	ktDead          // a dead key
	ktCons          // a switch to a console
	ktCur           // a cursor key
	ktShift         // a modifier
	ktMeta          // Alt with a character
	ktASCII         // a digit of a code typed with Alt held
	ktLock          // a modifier lock
	ktLetter        // a letter, which Caps Lock shifts
	ktSLock         // a sticky modifier
	ktDead2         // a dead key by its character
	ktBrl           // a braille dot
	ktCSI           // a CSI ~ key that says its modifiers (kernel 7.1 and later)
)

// Modifier groups, the bits of the shift state (linux/keyboard.h)
const (
	kgShift = iota
	kgAltGr
	kgCtrl
	kgAlt
	kgShiftL
	kgShiftR
	kgCtrlL
	kgCtrlR
	kgCapsShift
	nrShift
)

// The lock flags, as KDGKBLED reports them
const (
	ledScroll = 1 << iota
	ledNum
	ledCaps
)

const (
	nrKeys     = 256    // keycodes a keymap maps
	kNoSuchMap = 0x027f // KDGKBENT's answer for the first key of a map the keymap lacks
	symHole    = 0xf200 // K_HOLE as the kernel keeps it: a key that does nothing
)

// linuxKeymap is a Linux console keymap as KDGKBENT, KDGKBSENT and
// KDGKBDIACRUC read it
type linuxKeymap struct {
	maps    [256]*[nrKeys]uint16 // by shift state, nil where the keymap has none
	funcs   [256]string          // the function keys' strings
	accents []linuxAccent        // what a dead key makes of the character after it
	escMeta bool                 // Meta keys type ESC and the character (K_ESCPREFIX), else it with bit 7 set
	csiMods bool                 // cursor and CSI keys say their modifiers, as from kernel 7.1
}

// linuxAccent is an accented character: a dead key's character and the
// character typed after it
type linuxAccent struct {
	diacr, base, result rune
}

// linuxKeys translates keycodes as the Linux console does in Unicode mode,
// keeping its modifiers, locks, lock flags and dead keys. Codes typed with
// Alt and the keypad and braille type nothing; the keypad and cursor keys
// type as they do outside application mode, which the package never sets.
type linuxKeys struct {
	km        *linuxKeymap
	down      [nrKeys / 64]uint64 // keys down
	shiftDown [nrShift]uint8      // keys down of each modifier
	shift     uint16              // modifiers down
	lock      uint16              // modifiers locked
	slock     uint16              // modifiers stuck for the next key
	leds      uint8               // lock flags
	diacr     rune                // a dead key's character, for the next character
	deadNext  bool                // compose: the next character is a dead key's
}

// seed starts translation from the console's state: the modifiers it has
// down and its lock flags; keys already down are not known
func (k *linuxKeys) seed(shift, leds uint8) {
	*k = linuxKeys{km: k.km, leds: leds & (ledScroll | ledNum | ledCaps)}
	for g := range nrShift {
		if shift&(1<<g) != 0 {
			k.shiftDown[g], k.shift = 1, k.shift|1<<g
		}
	}
}

// isDown is whether a key is down
func (k *linuxKeys) isDown(code int) bool {
	return code < nrKeys && k.down[code/64]&(1<<(code%64)) != 0
}

// key translates one keycode event: down for a press, rep for a repeat. It
// returns what the console types for it and what it asks of the console.
func (k *linuxKeys) key(code int, down, rep bool) (typed []byte, act consoleAct) {
	if code >= nrKeys {
		return nil, act // past every keymap
	}
	if down {
		k.down[code/64] |= 1 << (code % 64)
	} else {
		k.down[code/64] &^= 1 << (code % 64)
	}

	state := ((k.shift | k.slock) ^ k.lock) & 0xff
	km := k.km.maps[state]
	// A key that says its modifiers falls back to the plain map when the
	// modifiers have no map: the kernel compares the entry with the bare
	// K_HOLE, which no kept entry is, so a hole in a map stays a hole
	if km == nil && state != 0 && k.km.csiMods && k.km.maps[0] != nil {
		if t := symType(k.km.maps[0][code]); t == ktCur || t == ktCSI {
			km = k.km.maps[0]
		}
	}
	if km == nil {
		k.computeShift()
		k.slock = 0
		return nil, act
	}

	sym := km[code]
	if sym < 0xf000 {
		if down {
			typed = k.unicode(typed, rune(sym))
		}
		return typed, act
	}
	t, val := symType(sym), byte(sym)
	if t == ktLetter {
		t = ktLatin
		// The kernel takes the shifted entry's value, whatever its type
		if k.leds&ledCaps != 0 {
			if m := k.km.maps[state^1<<kgShift]; m != nil {
				val = byte(m[code])
			}
		}
	}

	switch t {
	case ktLatin:
		if down {
			typed = k.unicode(typed, rune(val))
		}
	case ktFn:
		if down {
			typed = append(typed, k.km.funcs[val]...)
		}
	case ktSpec:
		if down {
			typed, act = k.spec(val, rep)
		}
	case ktPad:
		if down {
			typed = k.pad(val)
		}
	case ktDead:
		if down && int(val) < len(deadChars) {
			typed = k.dead(typed, rune(deadChars[val]))
		}
	case ktDead2:
		if down {
			typed = k.dead(typed, rune(val))
		}
	case ktCons:
		if down {
			act.vt = int(val) + 1
		}
	case ktCur:
		if down {
			typed = k.cur(val)
		}
	case ktShift:
		k.shiftKey(val, down, rep)
	case ktMeta:
		if down && k.km.escMeta {
			typed = append(typed, 0x1b, val)
		} else if down {
			typed = append(typed, val|0x80)
		}
	case ktLock:
		if down && !rep && val < 16 {
			k.lock ^= 1 << val
		}
	case ktSLock:
		k.shiftKey(val, down, rep)
		if down && !rep && val < 16 {
			k.slock ^= 1 << val
			if k.km.maps[(k.lock^k.slock)&0xff] == nil {
				k.slock = 1 << val
			}
		}
	case ktCSI:
		if down && k.km.csiMods {
			typed = k.csi(val)
		}
	}
	if t != ktSLock {
		k.slock = 0
	}
	return typed, act
}

// deadChars are the characters of the dead keys KT_DEAD names by number, as
// the accent table knows them
const deadChars = "`'^~\",_U.*=cki#o!?+-)(:n;$@"

// unicode types ch, or what a dead key before it makes of it; after compose,
// ch is the dead key
func (k *linuxKeys) unicode(typed []byte, ch rune) []byte {
	if k.diacr != 0 {
		ch, typed = k.combine(typed, ch)
	}
	if k.deadNext {
		k.deadNext, k.diacr = false, ch
		return typed
	}
	return utf8.AppendRune(typed, ch)
}

// dead is a dead key: the next character takes its accent; one already
// pending combines with it first
func (k *linuxKeys) dead(typed []byte, ch rune) []byte {
	if k.diacr != 0 {
		ch, typed = k.combine(typed, ch)
	}
	k.diacr = ch
	return typed
}

// combine is what the pending dead key makes of ch: their accented character,
// the dead key's own for space or itself, else ch with the dead key's
// character typed before it
func (k *linuxKeys) combine(typed []byte, ch rune) (rune, []byte) {
	d := k.diacr
	k.diacr = 0
	for _, a := range k.km.accents {
		if a.diacr == d && a.base == ch {
			return a.result, typed
		}
	}
	if ch == ' ' || ch == d {
		return d, typed
	}
	return ch, utf8.AppendRune(typed, d)
}

// symType is a typed keysym's type, -1 for a Unicode one
func symType(sym uint16) int {
	if sym < 0xf000 {
		return -1
	}
	return int(sym>>8) - 0xf0
}

// shiftKey moves a modifier, which a second key of the same group holds down
// until both are up; Caps Shift is Shift that ends Caps Lock
func (k *linuxKeys) shiftKey(val byte, down, rep bool) {
	if rep || val >= nrShift {
		return
	}
	if val == kgCapsShift {
		val = kgShift
		if down {
			k.leds &^= ledCaps // the kernel ends Caps Lock itself, raw or not
		}
	}
	switch {
	case down && k.shiftDown[val] < 255:
		k.shiftDown[val]++
	case !down && k.shiftDown[val] > 0:
		k.shiftDown[val]--
	}
	if k.shiftDown[val] > 0 {
		k.shift |= 1 << val
	} else {
		k.shift &^= 1 << val
	}
}

// computeShift counts the modifiers again from the keys down, by the plain
// map, as the kernel does when a key finds no map
func (k *linuxKeys) computeShift() {
	k.shift, k.shiftDown = 0, [nrShift]uint8{}
	plain := k.km.maps[0]
	if plain == nil {
		return
	}
	for code := range nrKeys {
		if !k.isDown(code) {
			continue
		}
		t, val := symType(plain[code]), byte(plain[code])
		if t != ktShift && t != ktSLock {
			continue
		}
		if val == kgCapsShift {
			val = kgShift
		}
		if val < nrShift {
			k.shiftDown[val]++
			k.shift |= 1 << val
		}
	}
}

// spec runs a console function key (KT_SPEC) as the console does cooked.
// The register and memory dumps, scrollback, Ctrl+Alt+Del, spawning a console
// and the last console do nothing; SAK the kernel runs itself, raw or not.
// Scroll Lock only ever restarts output, so a game can't be frozen.
func (k *linuxKeys) spec(val byte, rep bool) ([]byte, consoleAct) {
	var act consoleAct
	switch val {
	case 0: // K_HOLE
		k.computeShift()
	case 1: // K_ENTER, after a dead key pending
		var typed []byte
		if k.diacr != 0 {
			typed, k.diacr = utf8.AppendRune(typed, k.diacr), 0
		}
		return append(typed, '\r'), act
	case 5: // K_BREAK, which a raw tty reads as NUL
		return []byte{0}, act
	case 7: // K_CAPS
		if !rep {
			k.leds ^= ledCaps
			act.leds = true
		}
	case 8, 19: // K_NUM, K_BARENUMLOCK
		if !rep {
			k.leds ^= ledNum
			act.leds = true
		}
	case 9: // K_HOLD
		act.start = !rep
	case 13: // K_CAPSON
		if !rep && k.leds&ledCaps == 0 {
			k.leds |= ledCaps
			act.leds = true
		}
	case 14: // K_COMPOSE
		k.deadNext = true
	case 16: // K_DECRCONSOLE
		act.vt = vtPrev
	case 17: // K_INCRCONSOLE
		act.vt = vtNext
	}
	return nil, act
}

// padChars are what the keypad keys type with Num Lock on (K_P0 to K_PPARENR)
const padChars = "0123456789+-*/\r,.?()"

// The function strings the keypad types with Num Lock off
const (
	fnFind   = 20
	fnInsert = 21
	fnRemove = 22
	fnSelect = 23
	fnPgUp   = 24
	fnPgDn   = 25
)

// pad is a keypad key: its character with Num Lock on, else the navigation
// key it doubles as
func (k *linuxKeys) pad(val byte) []byte {
	if k.leds&ledNum == 0 {
		switch val {
		case 15, 16: // K_PCOMMA, K_PDOT
			return []byte(k.km.funcs[fnRemove])
		case 0:
			return []byte(k.km.funcs[fnInsert])
		case 1:
			return []byte(k.km.funcs[fnSelect])
		case 2:
			return k.cur(0)
		case 3:
			return []byte(k.km.funcs[fnPgDn])
		case 4:
			return k.cur(1)
		case 5:
			return []byte("\x1b[G")
		case 6:
			return k.cur(2)
		case 7:
			return []byte(k.km.funcs[fnFind])
		case 8:
			return k.cur(3)
		case 9:
			return []byte(k.km.funcs[fnPgUp])
		}
	}
	if int(val) < len(padChars) {
		return []byte{padChars[val]}
	}
	return nil
}

// cur is a cursor key (K_DOWN, K_LEFT, K_RIGHT, K_UP): CSI letter, or with
// modifiers down on a kernel that says them, CSI 1 ; modifiers letter
func (k *linuxKeys) cur(val byte) []byte {
	if val > 3 {
		return nil
	}
	ch := "BDCA"[val]
	if mod := k.csiMod(); mod > 1 && k.km.csiMods {
		return []byte{0x1b, '[', '1', ';', '0' + byte(mod), ch}
	}
	return []byte{0x1b, '[', ch}
}

// csi is a KT_CSI key: CSI value ~, or CSI value ; modifiers ~
func (k *linuxKeys) csi(val byte) []byte {
	if val > 99 {
		return nil
	}
	typed := strconv.AppendInt([]byte{0x1b, '['}, int64(val), 10)
	if mod := k.csiMod(); mod > 1 {
		typed = append(typed, ';', '0'+byte(mod))
	}
	return append(typed, '~')
}

// csiMod is xterm's modifier parameter for the modifiers down: Shift, Alt and
// Ctrl, the kernel leaving out their left and right groups and AltGr, which
// keymaps use to pick a layout
func (k *linuxKeys) csiMod() int {
	mod := 1
	if k.shift&(1<<kgShift) != 0 {
		mod++
	}
	if k.shift&(1<<kgAlt) != 0 {
		mod += 2
	}
	if k.shift&(1<<kgCtrl) != 0 {
		mod += 4
	}
	return mod
}

// mediumRaw decodes the Linux console's K_MEDIUMRAW input: a keycode under
// 128 is one byte, bit 7 set on release; a higher one is a byte of 0, 0x80 on
// release, then its high and low 7 bits, each with bit 7 set
type mediumRaw struct {
	up   bool
	high int
	n    int // bytes of a long keycode seen
}

// decode reports each keycode event in data, keeping a long keycode begun at
// its end for the next call
func (m *mediumRaw) decode(data []byte, key func(code int, down bool)) {
	for _, b := range data {
		if m.n > 0 && b&0x80 != 0 {
			if m.n++; m.n == 2 {
				m.high = int(b & 0x7f)
				continue
			}
			m.n = 0
			key(m.high<<7|int(b&0x7f), !m.up)
			continue
		}
		m.n = 0 // a long keycode broken off is dropped
		if b&0x7f == 0 {
			m.up, m.n = b != 0, 1
			continue
		}
		key(int(b&0x7f), b&0x80 == 0)
	}
}
