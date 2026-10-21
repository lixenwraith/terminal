package terminal

import "unicode/utf8"

// The FreeBSD console's keymap and its translation of keycodes, after the
// kernel's own (sys/dev/kbd/kbd.c, sys/dev/vt/vt_core.c), so a vt(4)
// keyboard switched to keycodes (K_CODE) types what the console types with it
// cooked: the keyboard's driver makes an action of each keycode by the keymap
// and its modifiers and locks, and vt types the action.

// Actions the keymap's special entries hold (sys/kbio.h)
const (
	fbsdNOP   = 0x00 // nothing
	fbsdLSH   = 0x02 // left Shift
	fbsdRSH   = 0x03 // right Shift
	fbsdCLK   = 0x04 // Caps Lock
	fbsdNLK   = 0x05 // Num Lock
	fbsdSLK   = 0x06 // Scroll Lock
	fbsdLALT  = 0x07 // left Alt
	fbsdBTAB  = 0x08 // back tab
	fbsdLCTR  = 0x09 // left Ctrl
	fbsdNEXT  = 0x0a // the next console
	fbsdFSCR  = 0x0b // console 1, the others after it
	fbsdFFN   = 0x1b // function key 1, the others after it
	fbsdLFN   = 0x7a // the last function key
	fbsdRCTR  = 0x80 // right Ctrl
	fbsdRALT  = 0x81 // right Alt
	fbsdALK   = 0x82 // AltGr Lock
	fbsdASH   = 0x83 // AltGr
	fbsdMETA  = 0x84 // Meta
	fbsdRBT   = 0x85 // reboot
	fbsdDBG   = 0x86 // the debugger
	fbsdSUSP  = 0x87 // suspend
	fbsdSPSC  = 0x88 // the screen saver
	fbsdFACC  = 0x89 // the first dead key
	fbsdLACC  = 0x97 // the last dead key
	fbsdSTBY  = 0x98 // standby
	fbsdPREV  = 0x99 // the previous console
	fbsdPNC   = 0x9a // panic
	fbsdLSHA  = 0x9b // left Shift, AltGr Lock pressed alone
	fbsdRSHA  = 0x9c // right Shift, AltGr Lock pressed alone
	fbsdLCTRA = 0x9d // left Ctrl, AltGr Lock pressed alone
	fbsdRCTRA = 0x9e // right Ctrl, AltGr Lock pressed alone
	fbsdLALTA = 0x9f // left Alt, AltGr Lock pressed alone
	fbsdRALTA = 0xa0 // right Alt, AltGr Lock pressed alone
	fbsdHALT  = 0xa1 // halt
	fbsdPDWN  = 0xa2 // power down
)

// What the keyboard's driver flags an action with (sys/kbio.h)
const (
	fbsdNoKey   = 0x01000000 // NOKEY: nothing to type
	fbsdFKey    = 0x02000000 // FKEY: a function key
	fbsdMKey    = 0x04000000 // MKEY: typed with Meta
	fbsdBKey    = 0x08000000 // BKEY: back tab
	fbsdErrKey  = 0x20000000 // ERRKEY: a character a dead key can't accent
	fbsdRelKey  = 0x40000000 // RELKEY: a special key let go
	fbsdSpclKey = 0x80000000 // SPCLKEY: a special action
	fbsdFlags   = 0xff000000 // KEYFLAGS
)

// The keyboard's state: the locks, as KDGKBSTATE reports them, and the
// modifiers (sys/dev/kbd/kbdreg.h)
const (
	fbsdCLKED    = 1 << 0 // Caps Lock on
	fbsdNLKED    = 1 << 1 // Num Lock on
	fbsdSLKED    = 1 << 2 // Scroll Lock on
	fbsdALKED    = 1 << 3 // AltGr Lock on
	fbsdLockMask = fbsdCLKED | fbsdNLKED | fbsdSLKED | fbsdALKED
	fbsdShifts1  = 1 << 16
	fbsdShifts2  = 1 << 17
	fbsdShifts   = fbsdShifts1 | fbsdShifts2
	fbsdCtls1    = 1 << 18
	fbsdCtls2    = 1 << 19
	fbsdCtls     = fbsdCtls1 | fbsdCtls2
	fbsdAlts1    = 1 << 20
	fbsdAlts2    = 1 << 21
	fbsdAlts     = fbsdAlts1 | fbsdAlts2
	fbsdAgrs1    = 1 << 22
	fbsdAgrs2    = 1 << 23
	fbsdAgrs     = fbsdAgrs1 | fbsdAgrs2
	fbsdMetas1   = 1 << 24
	fbsdMetas2   = 1 << 25
	fbsdMetas    = fbsdMetas1 | fbsdMetas2
	fbsdNLKDown  = 1 << 26
	fbsdSLKDown  = 1 << 27
	fbsdCLKDown  = 1 << 28
	fbsdALKDown  = 1 << 29
	fbsdShiftAOn = 1 << 30 // a Shift, Ctrl or Alt that locks AltGr pressed alone so far
)

const (
	fbsdNumKeys   = 256 // NUM_KEYS: entries in a keymap
	fbsdAltGrKeys = 128 // ALTGR_OFFSET: where the keys' AltGr entries start
	fbsdDeadKeys  = 15  // NUM_DEADKEYS
	fbsdAccents   = 52  // NUM_ACCENTCHARS: the characters a dead key accents
	fbsdLockC     = 1   // FLAG_LOCK_C: Caps Lock shifts the key
	fbsdLockN     = 2   // FLAG_LOCK_N: Num Lock shifts the key
	fbsdKeycodes  = 128 // K_CODE's keycodes, 7 bits
	fbsdWindows   = 12  // VT_MAXWINDOWS: the consoles vt(4) has
)

// fbsdKeymap is a FreeBSD console keymap as GIO_KEYMAP and GIO_DEADKEYMAP
// read it, and whether vt(4) takes right Alt for AltGr
// (kern.vt.enable_altgr) rather than the Alt that types ESC before a
// character
type fbsdKeymap struct {
	keys     [fbsdNumKeys]fbsdKey
	accents  [fbsdDeadKeys]fbsdAccent
	nAccents int
	altGr    bool
}

// fbsdKey is a key's entry (keyent_t), laid out as the kernel's: its action
// in each state of Shift, Ctrl and Alt, the states whose action is special,
// and the locks that shift it
type fbsdKey struct {
	actions [8]uint32
	spcl    uint8
	flgs    uint8
}

// fbsdAccent is a dead key's accent (acc_t), laid out as the kernel's: its
// character, and the characters it makes of others, in pairs up to a zero
type fbsdAccent struct {
	char  uint32
	pairs [fbsdAccents][2]uint32
}

// fbsdKeys translates keycodes as a vt(4) console does, keeping the
// keyboard's modifiers, locks and dead key, the special action each key down
// began, the code left Alt and the keypad compose, and Alt for vt's ESC. The
// machine's keys (reboot, halt, power down, suspend, standby, the debugger,
// panic), the screen saver and paste type nothing; Scroll Lock does nothing,
// leaving the console's history mode alone. The cursor keys type as they do
// outside application mode, which the package never sets.
type fbsdKeys struct {
	km        *fbsdKeymap
	down      [fbsdKeycodes / 64]uint64 // keys down
	state     uint32                    // modifiers and locks
	accent    int                       // the dead key pending, from 1; zero for none
	lastact   [fbsdKeycodes]uint32      // the special action each key down began
	composing bool                      // left Alt down, the keypad's digits composing a code
	composed  uint32                    // the code composed so far
	alt       bool                      // Alt down, for vt's ESC before a character
}

// seed starts translation from the console's locks; modifiers already down
// are not known, the keyboard's driver forgetting them as it switches
func (k *fbsdKeys) seed(locks uint32) {
	*k = fbsdKeys{km: k.km, state: locks & fbsdLockMask}
}

// isDown is whether a key is down
func (k *fbsdKeys) isDown(code int) bool {
	return code >= 0 && code < fbsdKeycodes && k.down[code/64]&(1<<(code%64)) != 0
}

// key translates one keycode event: down for a press, or a repeat, which the
// keyboard sends as another press. It returns what the console types for it
// and what it asks of the console.
func (k *fbsdKeys) key(code int, down bool) (typed []byte, act consoleAct) {
	if code <= 0 || code >= fbsdKeycodes {
		return nil, act
	}
	if down {
		k.down[code/64] |= 1 << (code % 64)
	} else {
		k.down[code/64] &^= 1 << (code % 64)
	}
	if k.compose(code, down) {
		return nil, act
	}
	locks := k.state & fbsdLockMask
	c := k.action(code, !down)
	act.leds = k.state&fbsdLockMask != locks
	typed, act.vt = k.vt(c)
	if !k.composing && k.composed > 0 {
		// Left Alt up types the code composed
		ch, _ := k.vt(k.composed)
		typed, k.composed = append(typed, ch...), 0
	}
	return typed, act
}

// The keycodes of left Alt and the keypad's digits, which the keyboard's
// driver composes a code with
const fbsdKeyLeftAlt = 0x38

var fbsdPadDigits = map[int]uint32{
	0x47: 7, 0x48: 8, 0x49: 9, 0x4b: 4, 0x4c: 5, 0x4d: 6, 0x4f: 1, 0x50: 2, 0x51: 3, 0x52: 0,
}

// compose composes a code of the keypad's digits typed with left Alt down, as
// the keyboard's driver does before it makes an action of a keycode
// (kbdmux_read_char): true when it takes the key, which then types nothing.
// Another key ends a code begun, and is lost to it; a code past a byte types
// nothing.
func (k *fbsdKeys) compose(code int, down bool) bool {
	if code == fbsdKeyLeftAlt {
		if down && !k.composing {
			k.composing, k.composed = true, 0
		} else if !down && k.composing {
			k.composing = false
			if k.composed > 0xff {
				k.composed = 0
			}
		}
		return false
	}
	if !k.composing {
		return false
	}
	if digit, ok := fbsdPadDigits[code]; ok {
		if down {
			k.composed = min(k.composed*10+digit, 0x100)
		}
		return true
	}
	if k.composed > 0 {
		k.composing, k.composed = false, 0
		return true
	}
	return false
}

// lockKey toggles a lock as its key goes down, not as it repeats
func lockKey(state *uint32, down, locked uint32) {
	if *state&down == 0 {
		*state |= down
		*state ^= locked
	}
}

// action makes an action of a keycode by the keymap, as the keyboard's
// driver does (genkbd_keyaction): a character, a special action, or a flag
// for none
func (k *fbsdKeys) action(code int, up bool) uint32 {
	state := k.state
	i := code
	if f := state & (fbsdAgrs | fbsdALKED); f == fbsdAgrs1 || f == fbsdAgrs2 || f == fbsdALKED {
		i += fbsdAltGrKeys
	}
	key := &k.km.keys[i]
	col := 0
	if state&fbsdShifts != 0 {
		col |= 1
	}
	if state&fbsdCtls != 0 {
		col |= 2
	}
	if state&fbsdAlts != 0 {
		col |= 4
	}
	if key.flgs&fbsdLockC != 0 && state&fbsdCLKED != 0 || key.flgs&fbsdLockN != 0 && state&fbsdNLKED != 0 {
		col ^= 1
	}

	if up {
		action := k.lastact[code]
		k.lastact[code] = fbsdNOP
		if alone, ok := fbsdLockingMods[action]; ok {
			// Pressed alone, the key toggles AltGr Lock
			if state&fbsdShiftAOn != 0 {
				lockKey(&state, fbsdALKDown, fbsdALKED)
				state &^= fbsdALKDown
			}
			action = alone
		}
		switch action {
		case fbsdNOP:
			// Regular keys go up unreported
			k.state &^= fbsdShiftAOn
			return fbsdNoKey
		case fbsdLSH:
			state &^= fbsdShifts1
		case fbsdRSH:
			state &^= fbsdShifts2
		case fbsdLCTR:
			state &^= fbsdCtls1
		case fbsdRCTR:
			state &^= fbsdCtls2
		case fbsdLALT:
			state &^= fbsdAlts1
		case fbsdRALT:
			state &^= fbsdAlts2
		case fbsdASH:
			state &^= fbsdAgrs1
		case fbsdMETA:
			state &^= fbsdMetas1
		case fbsdNLK:
			state &^= fbsdNLKDown
		case fbsdCLK:
			state &^= fbsdCLKDown
		case fbsdSLK:
			state &^= fbsdSLKDown
		case fbsdALK:
			state &^= fbsdALKDown
		}
		k.state = state &^ fbsdShiftAOn
		return fbsdSpclKey | fbsdRelKey | action
	}

	action := key.actions[col]
	state &^= fbsdShiftAOn
	if key.spcl&(0x80>>col) == 0 {
		// A character, the dead key pending accenting it
		k.lastact[code] = fbsdNOP
		k.state = state
		if k.accent > 0 {
			if action = k.accented(action); action == fbsdErrKey {
				return action
			}
		}
		if state&fbsdMetas != 0 {
			action |= fbsdMKey
		}
		return action
	}

	// A special key keeps the action it went down with while it repeats
	if k.lastact[code] == fbsdNOP {
		k.lastact[code] = action
	}
	if k.lastact[code] != action {
		action = fbsdNOP
	}
	if alone, ok := fbsdLockingMods[action]; ok {
		state |= fbsdShiftAOn
		action = alone
	}
	switch action {
	case fbsdNLK:
		lockKey(&state, fbsdNLKDown, fbsdNLKED)
	case fbsdCLK:
		lockKey(&state, fbsdCLKDown, fbsdCLKED)
	case fbsdSLK:
		// The console's history mode follows Scroll Lock; raw, it stays
	case fbsdALK:
		lockKey(&state, fbsdALKDown, fbsdALKED)
	case fbsdSPSC, fbsdRBT, fbsdSUSP, fbsdSTBY, fbsdDBG, fbsdNEXT, fbsdPREV, fbsdPNC, fbsdHALT, fbsdPDWN:
		k.accent = 0
	case fbsdBTAB:
		k.accent = 0
		action |= fbsdBKey
	case fbsdLSH:
		state |= fbsdShifts1
	case fbsdRSH:
		state |= fbsdShifts2
	case fbsdLCTR:
		state |= fbsdCtls1
	case fbsdRCTR:
		state |= fbsdCtls2
	case fbsdLALT:
		state |= fbsdAlts1
	case fbsdRALT:
		state |= fbsdAlts2
	case fbsdASH:
		state |= fbsdAgrs1
	case fbsdMETA:
		state |= fbsdMetas1
	case fbsdNOP:
		k.state = state
		return fbsdNoKey
	default:
		k.state = state
		if action >= fbsdFACC && action <= fbsdLACC {
			action = k.deadKey(action)
			if action == fbsdNoKey || action == fbsdErrKey {
				return action
			}
			if state&fbsdMetas != 0 {
				return action | fbsdMKey
			}
			return action
		}
		if k.accent > 0 {
			k.accent = 0
			return fbsdErrKey
		}
		if action >= fbsdFFN && action <= fbsdLFN {
			action |= fbsdFKey
		}
		return fbsdSpclKey | action
	}
	k.state = state
	return fbsdSpclKey | action
}

// fbsdLockingMods are the modifiers that lock AltGr pressed alone, and the
// modifier each is otherwise
var fbsdLockingMods = map[uint32]uint32{
	fbsdLSHA: fbsdLSH, fbsdRSHA: fbsdRSH, fbsdLCTRA: fbsdLCTR,
	fbsdRCTRA: fbsdRCTR, fbsdLALTA: fbsdLALT, fbsdRALTA: fbsdRALT,
}

// deadKey is a dead key: the next character takes its accent, and the dead
// key again types the accent itself
func (k *fbsdKeys) deadKey(action uint32) uint32 {
	i := int(action-fbsdFACC) + 1
	if i > k.km.nAccents || k.km.accents[i-1].char == 0 {
		k.accent = 0
		return fbsdErrKey
	}
	if i == k.accent {
		k.accent = 0
		return k.km.accents[i-1].char
	}
	k.accent = i
	return fbsdNoKey
}

// accented is what the dead key pending makes of ch: its accent for a space,
// the accented character, or an error for a character it can't accent
func (k *fbsdKeys) accented(ch uint32) uint32 {
	acc := &k.km.accents[k.accent-1]
	k.accent = 0
	if ch == ' ' {
		return acc.char
	}
	for _, p := range acc.pairs {
		if p[0] == 0 {
			break
		}
		if p[0] == ch {
			return p[1]
		}
	}
	return fbsdErrKey
}

// vt types an action as vt(4) does (vt_processkey): a character as UTF-8,
// after an ESC while Alt is down; F1 to F12, the cursor and editing keys and
// back tab as xterm's sequences; and a console to switch to
func (k *fbsdKeys) vt(c uint32) (typed []byte, vt int) {
	if c&fbsdRelKey != 0 {
		switch c &^ fbsdRelKey {
		case fbsdSpclKey | fbsdRALT:
			if k.km.altGr {
				break
			}
			fallthrough
		case fbsdSpclKey | fbsdLALT:
			k.alt = false
		}
		return nil, 0
	}
	switch c {
	case fbsdSpclKey | fbsdRALT:
		if !k.km.altGr {
			k.alt = true
		}
	case fbsdSpclKey | fbsdLALT:
		k.alt = true
	}
	if c&fbsdSpclKey != 0 {
		c &^= fbsdSpclKey
		switch {
		case c >= fbsdFSCR && c < fbsdFSCR+fbsdWindows:
			return nil, int(c-fbsdFSCR) + 1
		case c == fbsdNEXT:
			return nil, vtNext
		case c == fbsdPREV:
			return nil, vtPrev
		case c == fbsdBKey|fbsdBTAB:
			return []byte("\x1b[Z"), 0
		case c&fbsdFKey != 0:
			return fbsdFunctionKey(c &^ fbsdFKey), 0
		}
		return nil, 0
	}
	if c&fbsdFlags != 0 {
		return nil, 0 // nothing, a character it can't accent, or with Meta, which vt drops
	}
	if k.alt {
		typed = append(typed, 0x1b)
	}
	return utf8.AppendRune(typed, rune(c)), 0
}

// fbsdFunctionKeys are what vt(4) types for the function keys, by number:
// xterm's sequences for F1 to F12, and for the cursor and editing keys the
// keymap binds as function keys 49 to 61; nothing for the others
var fbsdFunctionKeys = map[uint32]string{
	1: "\x1bOP", 2: "\x1bOQ", 3: "\x1bOR", 4: "\x1bOS",
	5: "\x1b[15~", 6: "\x1b[17~", 7: "\x1b[18~", 8: "\x1b[19~",
	9: "\x1b[20~", 10: "\x1b[21~", 11: "\x1b[23~", 12: "\x1b[24~",
	49: "\x1b[H", 50: "\x1b[A", 51: "\x1b[5~", 53: "\x1b[D", 55: "\x1b[C",
	57: "\x1b[F", 58: "\x1b[B", 59: "\x1b[6~", 60: "\x1b[2~", 61: "\x1b[3~",
}

// fbsdFunctionKey is what vt(4) types for a function key action
func fbsdFunctionKey(action uint32) []byte {
	if s, ok := fbsdFunctionKeys[action-fbsdFFN+1]; ok {
		return []byte(s)
	}
	return nil
}
