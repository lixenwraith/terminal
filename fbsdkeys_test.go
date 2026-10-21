package terminal

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// testFreeBSDKeymap is the FreeBSD kernel's built-in keymap (US) and accent
// map (sys/dev/kbd/kbdtables.h), as GIO_KEYMAP and GIO_DEADKEYMAP read them,
// with vt's default AltGr
func testFreeBSDKeymap(t *testing.T) *fbsdKeymap {
	t.Helper()
	f, err := os.Open("testdata/freebsd-keymap.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	km := &fbsdKeymap{nAccents: fbsdDeadKeys, altGr: true}
	hex := func(s string) uint32 {
		x, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		return uint32(x)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		i, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		switch fields[0] {
		case "key":
			key := &km.keys[i]
			for j, v := range fields[2:10] {
				key.actions[j] = hex(v)
			}
			key.spcl, key.flgs = uint8(hex(fields[10])), uint8(hex(fields[11]))
		case "acc":
			acc := &km.accents[i]
			acc.char = hex(fields[2])
			for j, pair := range fields[3:] {
				base, result, _ := strings.Cut(pair, ":")
				acc.pairs[j] = [2]uint32{hex(base), hex(result)}
			}
		}
	}
	return km
}

// The FreeBSD console's keycodes up to 88 are the Linux console's; its others
// are its own
const (
	fbsdKeyKPEnter   = 89
	fbsdKeyRightCtrl = 90
	fbsdKeyPrint     = 92
	fbsdKeyRightAlt  = 93
	fbsdKeyHome      = 94
	fbsdKeyUp        = 95
	fbsdKeyPageUp    = 96
	fbsdKeyLeft      = 97
	fbsdKeyEnd       = 99
	fbsdKeyInsert    = 102
	fbsdKeyDelete    = 103
	fbsdKeyPause     = 104
	fbsdKeyF11       = 87
	fbsdKeyF12       = 88
	keyKP9           = 73
	keyKP6           = 77
	keyKP2           = 80
	keyKP3           = 81
)

// fbsdTypedBy is what vt types for strokes, and what they ask of it
func fbsdTypedBy(k *fbsdKeys, strokes []stroke) (string, []consoleAct) {
	var out []byte
	var acts []consoleAct
	for _, s := range strokes {
		typed, act := k.key(s.code, s.down)
		out = append(out, typed...)
		if act != (consoleAct{}) {
			acts = append(acts, act)
		}
	}
	return string(out), acts
}

// Keys type what vt types for them cooked, by the keymap and the modifiers
// and locks they move: Shift into the character, Caps Lock on letters only,
// Ctrl as control characters, Alt as an ESC prefix, F1 to F12 and the cursor
// and editing keys as xterm's sequences, and the keypad by Num Lock
func TestFreeBSDConsoleTypesAsCooked(t *testing.T) {
	cases := []struct {
		name    string
		strokes []stroke
		want    string
		altGr   string // with right Alt as Alt, when it differs
	}{
		{"letter", press(keyA), "a", ""},
		{"shifted letter", hold(keyLeftShift, press(keyA)), "A", ""},
		{"shifted digit", hold(keyLeftShift, press(key1)), "!", ""},
		{"caps lock", slices.Concat(press(keyCapsLock), press(keyA, key1), press(keyCapsLock), press(keyA)), "A1a", ""},
		{"caps lock with shift", slices.Concat(press(keyCapsLock), hold(keyLeftShift, press(keyA))), "a", ""},
		{"ctrl letter", hold(keyLeftCtrl, press(keyA)), "\x01", ""},
		{"ctrl shift letter", hold(keyLeftCtrl, hold(keyLeftShift, press(keyA))), "\x01", ""},
		{"right ctrl letter", hold(fbsdKeyRightCtrl, press(keyA)), "\x01", ""},
		{"ctrl space", hold(keyLeftCtrl, press(keySpace)), "\x00", ""},
		{"ctrl backspace", hold(keyLeftCtrl, press(keyBackspace)), "\x7f", ""},
		{"alt letter", hold(keyLeftAlt, press(keyA)), "\x1ba", ""},
		{"alt enter", hold(keyLeftAlt, press(keyEnter)), "\x1b\r", ""},
		{"ctrl alt letter", hold(keyLeftCtrl, hold(keyLeftAlt, press(keyA))), "\x1b\x01", ""},
		{"right alt letter", hold(fbsdKeyRightAlt, press(keyA)), "a", "\x1ba"},
		{"alt released", slices.Concat(hold(keyLeftAlt, press(keyA)), press(keyA)), "\x1baa", ""},
		{"enter tab escape backspace", press(keyEnter, keyTab, keyEsc, keyBackspace), "\r\t\x1b\x08", ""},
		{"back tab", hold(keyLeftShift, press(keyTab)), "\x1b[Z", ""},
		{"alt back tab", hold(keyLeftAlt, hold(keyLeftShift, press(keyTab))), "\x1b[Z", ""},
		{"function keys", press(keyF1, keyF6, fbsdKeyF12), "\x1bOP\x1b[17~\x1b[24~", ""},
		{"shifted function key, no sequence", hold(keyLeftShift, press(keyF1)), "", ""},
		{"editing keys", press(fbsdKeyHome, fbsdKeyEnd, fbsdKeyInsert, fbsdKeyDelete, fbsdKeyPageUp), "\x1b[H\x1b[F\x1b[2~\x1b[3~\x1b[5~", ""},
		{"cursor", press(fbsdKeyUp, fbsdKeyLeft), "\x1b[A\x1b[D", ""},
		{"shifted cursor", hold(keyLeftShift, press(fbsdKeyUp)), "\x1b[A", ""},
		{"keypad, num lock off", press(keyKP8, keyKP5, keyKP0, keyKPDot, fbsdKeyKPEnter), "\x1b[A\x1b[2~\x7f\r", ""},
		{"keypad, num lock on", slices.Concat(press(keyNumLock), press(keyKP8, keyKP0, keyKPDot)), "80.", ""},
		{"repeat", []stroke{{keyQ, true}, {keyQ, true}, {keyQ, true}, {keyQ, false}}, "qqq", ""},
		{"shift released first", []stroke{{keyLeftShift, true}, {keyA, true}, {keyLeftShift, false}, {keyA, true}, {keyA, false}}, "Aa", ""},
		{"two shifts", []stroke{{keyLeftShift, true}, {keyRtShift, true}, {keyLeftShift, false}, {keyA, true}, {keyA, false}, {keyRtShift, false}}, "A", ""},
		{"alt keypad code", hold(keyLeftAlt, press(keyKP6, keyKP5)), "A", ""},
		{"alt keypad code, num lock on", slices.Concat(press(keyNumLock), hold(keyLeftAlt, press(keyKP2, keyKP3, keyKP3))), "é", ""},
		{"alt keypad code past a byte", hold(keyLeftAlt, press(keyKP9, keyKP9, keyKP9)), "", ""},
		{"another key ends a code", slices.Concat([]stroke{{keyLeftAlt, true}}, press(keyKP6, keyA, keyA), []stroke{{keyLeftAlt, false}}), "\x1ba", ""},
		{"right alt keypad", hold(fbsdKeyRightAlt, press(keyKP6)), "6", "\x1b6"},
		{"inert keys", slices.Concat(
			hold(keyLeftCtrl, hold(keyLeftAlt, press(fbsdKeyDelete))), // reboot
			hold(keyLeftCtrl, hold(keyLeftAlt, press(keyEsc))),        // the debugger
			hold(keyLeftShift, press(fbsdKeyInsert)),                  // paste
			hold(keyLeftAlt, press(fbsdKeyPause)),                     // suspend
			press(keyScrollLk, fbsdKeyPause)), "", ""},
	}
	for _, altGr := range []bool{true, false} {
		km := testFreeBSDKeymap(t)
		km.altGr = altGr
		for _, c := range cases {
			want := c.want
			if !altGr && c.altGr != "" {
				want = c.altGr
			}
			k := &fbsdKeys{km: km}
			got, _ := fbsdTypedBy(k, c.strokes)
			if got != want {
				t.Errorf("%s (altGr %v): typed %q, want %q", c.name, altGr, got, want)
			}
			if k.state&^fbsdLockMask != 0 || k.alt {
				t.Errorf("%s (altGr %v): modifiers %#x (alt %v) left down", c.name, altGr, k.state&^fbsdLockMask, k.alt)
			}
		}
	}
}

// Keys that switch consoles and the locks ask the console for what vt would
// do cooked: a console by number or the one before or after, and the locks
// kept with the keyboard. Scroll Lock asks for nothing, the console's history
// mode staying off.
func TestFreeBSDConsoleKeysAskTheConsole(t *testing.T) {
	cases := []struct {
		name    string
		strokes []stroke
		want    []consoleAct
		locks   uint32
	}{
		{"alt f2", hold(keyLeftAlt, press(keyF2)), []consoleAct{{vt: 2}}, 0},
		{"ctrl alt f1", hold(keyLeftCtrl, hold(keyLeftAlt, press(keyF1))), []consoleAct{{vt: 1}}, 0},
		{"alt shift f1", hold(keyLeftAlt, hold(keyLeftShift, press(keyF1))), []consoleAct{{vt: 11}}, 0},
		{"alt f12", hold(keyLeftAlt, press(fbsdKeyF12)), []consoleAct{{vt: 12}}, 0},
		{"print screen, shifted", slices.Concat(press(fbsdKeyPrint), hold(keyLeftShift, press(fbsdKeyPrint))), []consoleAct{{vt: vtNext}, {vt: vtPrev}}, 0},
		{"caps lock twice", press(keyCapsLock, keyCapsLock), []consoleAct{{leds: true}, {leds: true}}, 0},
		{"caps lock held", []stroke{{keyCapsLock, true}, {keyCapsLock, true}, {keyCapsLock, false}}, []consoleAct{{leds: true}}, fbsdCLKED},
		{"num lock", press(keyNumLock), []consoleAct{{leds: true}}, fbsdNLKED},
		{"scroll lock", press(keyScrollLk), nil, 0},
		{"a special key repeats as it went down", []stroke{
			{keyLeftAlt, true}, {keyF2, true}, {keyLeftAlt, false}, {keyF2, true}, {keyF2, false},
		}, []consoleAct{{vt: 2}}, 0},
	}
	for _, c := range cases {
		k := &fbsdKeys{km: testFreeBSDKeymap(t)}
		if _, got := fbsdTypedBy(k, c.strokes); !slices.Equal(got, c.want) {
			t.Errorf("%s: asked %+v, want %+v", c.name, got, c.want)
		}
		if k.state != c.locks {
			t.Errorf("%s: state %#x, want %#x", c.name, k.state, c.locks)
		}
	}
}

// Translation starts from the console's locks, with no modifier down
func TestFreeBSDConsoleSeedsFromTheConsole(t *testing.T) {
	k := &fbsdKeys{km: testFreeBSDKeymap(t)}
	fbsdTypedBy(k, []stroke{{keyLeftShift, true}, {keyLeftAlt, true}})
	k.seed(fbsdNLKED | fbsdCLKED | fbsdShifts1)
	if got, _ := fbsdTypedBy(k, press(keyKP8, keyA)); got != "8A" {
		t.Errorf("typed %q, want %q", got, "8A")
	}
	if k.isDown(keyLeftShift) || k.state != fbsdNLKED|fbsdCLKED {
		t.Errorf("seeded state %#x, left shift down %v", k.state, k.isDown(keyLeftShift))
	}
}

// AltGr, from a key some keymaps bind, and AltGr Lock, from a modifier that
// toggles it pressed alone, type the keys' AltGr entries
func TestFreeBSDConsoleAltGr(t *testing.T) {
	km := testFreeBSDKeymap(t)
	all := func(action uint32) fbsdKey {
		return fbsdKey{actions: [8]uint32{action, action, action, action, action, action, action, action}, spcl: 0xff}
	}
	// AltGr looks every key up in the keymap's AltGr half, modifiers too
	km.keys[fbsdKeyRightAlt] = all(fbsdASH)
	km.keys[keyLeftShift] = all(fbsdLSHA)
	km.keys[fbsdAltGrKeys+keyLeftShift] = all(fbsdLSHA)
	km.keys[fbsdAltGrKeys+keyA] = fbsdKey{actions: [8]uint32{'α', 'Α', 1, 1, 'α', 'Α', 1, 1}, flgs: fbsdLockC}
	k := &fbsdKeys{km: km}
	strokes := slices.Concat(
		hold(fbsdKeyRightAlt, press(keyA)),
		press(keyA),
		press(keyLeftShift), // AltGr Lock on
		press(keyA),
		hold(keyLeftShift, press(keyA)), // Shift with a key leaves the lock
		press(keyLeftShift),             // AltGr Lock off
		press(keyA))
	got, acts := fbsdTypedBy(k, strokes)
	if got != "αaαΑa" {
		t.Errorf("typed %q, want %q", got, "αaαΑa")
	}
	if want := []consoleAct{{leds: true}, {leds: true}}; !slices.Equal(acts, want) {
		t.Errorf("asked %+v, want %+v", acts, want)
	}
	if k.state != 0 {
		t.Errorf("state %#x left", k.state)
	}
}

// Dead keys type what the accent map makes of the character after them, as
// vt does cooked: the accented character, the accent for a space or the dead
// key again, and nothing for a character the accent map has no pair for or a
// special key, which ends the dead key. Modifiers keep it pending.
func TestFreeBSDConsoleDeadKeys(t *testing.T) {
	km := testFreeBSDKeymap(t)
	km.keys[keyGrave].actions[0], km.keys[keyGrave].actions[1] = fbsdFACC, fbsdFACC+3 // grave, tilde
	km.keys[keyGrave].actions[2] = fbsdFACC + 4                                       // macron, with no accent
	km.keys[keyGrave].spcl |= 0x80 | 0x40 | 0x20
	cases := []struct {
		name    string
		strokes []stroke
		want    string
	}{
		{"dead key", press(keyGrave, keyA), "à"},
		{"shifted dead key", slices.Concat(hold(keyLeftShift, press(keyGrave)), press(keyN)), "ñ"},
		{"shifted character", slices.Concat(press(keyGrave), hold(keyLeftShift, press(keyA))), "À"},
		{"dead key and space", press(keyGrave, keySpace), "`"},
		{"dead key twice", press(keyGrave, keyGrave, keyA), "`a"},
		{"no pair", press(keyGrave, keyB, keyA), "a"},
		{"over a cursor key", press(keyGrave, fbsdKeyUp, keyA), "a"},
		{"no accent", slices.Concat(hold(keyLeftCtrl, press(keyGrave)), press(keyA)), "a"},
	}
	for _, c := range cases {
		k := &fbsdKeys{km: km}
		if got, _ := fbsdTypedBy(k, c.strokes); got != c.want {
			t.Errorf("%s: typed %q, want %q", c.name, got, c.want)
		}
	}
}
