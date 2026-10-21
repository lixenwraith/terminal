package terminal

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// testKeymap is the kernel's built-in keymap (US), as KDGKBENT and KDGKBSENT
// read it from a Linux 6.18 console
func testKeymap(t *testing.T) *linuxKeymap {
	t.Helper()
	f, err := os.Open("testdata/linux-keymap.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	km := &linuxKeymap{escMeta: true}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<16)
	for sc.Scan() {
		kind, rest, _ := strings.Cut(sc.Text(), " ")
		n, rest, _ := strings.Cut(rest, " ")
		i, err := strconv.Atoi(n)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "map":
			m := new([nrKeys]uint16)
			for j, v := range strings.Fields(rest) {
				x, err := strconv.ParseUint(v, 16, 16)
				if err != nil {
					t.Fatal(err)
				}
				m[j] = uint16(x) ^ 0xf000
			}
			km.maps[i] = m
		case "fn":
			if km.funcs[i], err = strconv.Unquote(rest); err != nil {
				t.Fatal(err)
			}
		}
	}
	return km
}

// Linux input keycodes
const (
	keyEsc       = 1
	key1         = 2
	key2         = 3
	keyBackspace = 14
	keyTab       = 15
	keyQ         = 16
	keyO         = 24
	keyEnter     = 28
	keyLeftCtrl  = 29
	keyA         = 30
	keyS         = 31
	keyGrave     = 41
	keyLeftShift = 42
	keyB         = 48
	keyN         = 49
	keyRtShift   = 54
	keyLeftAlt   = 56
	keySpace     = 57
	keyCapsLock  = 58
	keyF1        = 59
	keyF2        = 60
	keyF6        = 64
	keyNumLock   = 69
	keyScrollLk  = 70
	keyKP8       = 72
	keyKP5       = 76
	keyKP0       = 82
	keyKPDot     = 83
	keyKPEnter   = 96
	keyRightAlt  = 100
	keyHome      = 102
	keyUp        = 103
	keyPageUp    = 104
	keyLeft      = 105
	keyRight     = 106
	keyDelete    = 111
	keyPause     = 119
	keyCompose   = 127
)

// stroke is a key going down (repeating when already down) or up
type stroke struct {
	code int
	down bool
}

func press(codes ...int) []stroke {
	var s []stroke
	for _, c := range codes {
		s = append(s, stroke{c, true}, stroke{c, false})
	}
	return s
}

func hold(code int, inner ...[]stroke) []stroke {
	s := []stroke{{code, true}}
	for _, in := range inner {
		s = append(s, in...)
	}
	return append(s, stroke{code, false})
}

// typedBy is what the console types for strokes, and what they ask of it
func typedBy(k *linuxKeys, strokes []stroke) (string, []linuxAct) {
	var out []byte
	var acts []linuxAct
	for _, s := range strokes {
		typed, act := k.key(s.code, s.down, s.down && k.isDown(s.code))
		out = append(out, typed...)
		if act != (linuxAct{}) {
			acts = append(acts, act)
		}
	}
	return string(out), acts
}

// Keys type what the Linux console types for them cooked, by its keymap and
// the modifiers and locks they move: Shift into the character, Caps Lock on
// letters only, Ctrl as control characters, Alt as an ESC prefix, the
// function and editing keys as their strings, the keypad by Num Lock, and
// cursor keys with their modifiers only on a kernel that says them
func TestLinuxConsoleTypesAsCooked(t *testing.T) {
	cases := []struct {
		name    string
		strokes []stroke
		want    string // before kernel 7.1
		csiMods string // from 7.1, when it differs
	}{
		{"letter", press(keyA), "a", ""},
		{"shifted letter", hold(keyLeftShift, press(keyA)), "A", ""},
		{"shifted digit", hold(keyLeftShift, press(key1)), "!", ""},
		{"caps lock", slices.Concat(press(keyCapsLock), press(keyA, key1), press(keyCapsLock), press(keyA)), "A1a", ""},
		{"caps lock with shift", slices.Concat(press(keyCapsLock), hold(keyLeftShift, press(keyA))), "a", ""},
		{"ctrl letter", hold(keyLeftCtrl, press(keyA)), "\x01", ""},
		{"ctrl shift letter", hold(keyLeftCtrl, hold(keyLeftShift, press(keyA))), "\x01", ""},
		{"ctrl space", hold(keyLeftCtrl, press(keySpace)), "\x00", ""},
		{"ctrl backspace", hold(keyLeftCtrl, press(keyBackspace)), "\x08", ""},
		{"alt letter", hold(keyLeftAlt, press(keyA)), "\x1ba", ""},
		{"alt enter", hold(keyLeftAlt, press(keyEnter)), "\x1b\r", ""},
		{"ctrl alt letter", hold(keyLeftCtrl, hold(keyLeftAlt, press(keyA))), "\x1b\x01", ""},
		{"altgr", hold(keyRightAlt, press(key2)), "@", ""},
		{"enter tab escape backspace", press(keyEnter, keyTab, keyEsc, keyBackspace), "\r\t\x1b\x7f", ""},
		{"function keys", press(keyF1, keyF6), "\x1b[[A\x1b[17~", ""},
		{"shifted function key", hold(keyLeftShift, press(keyF1)), "\x1b[23~", ""},
		{"editing keys", press(keyHome, keyDelete, keyPageUp, keyPause), "\x1b[1~\x1b[3~\x1b[5~\x1b[P", ""},
		{"cursor", press(keyUp, keyLeft), "\x1b[A\x1b[D", ""},
		{"shifted cursor", hold(keyLeftShift, press(keyUp)), "\x1b[A", "\x1b[1;2A"},
		{"ctrl cursor", hold(keyLeftCtrl, press(keyRight)), "\x1b[C", "\x1b[1;5C"},
		{"alt shift cursor, no map", hold(keyLeftAlt, hold(keyLeftShift, press(keyUp))), "", "\x1b[1;4A"},
		{"keypad, num lock off", press(keyKP8, keyKP5, keyKP0, keyKPDot, keyKPEnter), "\x1b[A\x1b[G\x1b[2~\x1b[3~\r", ""},
		{"keypad, num lock on", slices.Concat(press(keyNumLock), press(keyKP8, keyKP0, keyKPDot)), "80.", ""},
		{"repeat", []stroke{{keyQ, true}, {keyQ, true}, {keyQ, true}, {keyQ, false}}, "qqq", ""},
		{"shift released first", []stroke{{keyLeftShift, true}, {keyA, true}, {keyLeftShift, false}, {keyA, true}, {keyA, false}}, "Aa", ""},
		{"two shifts", []stroke{{keyLeftShift, true}, {keyRtShift, true}, {keyLeftShift, false}, {keyA, true}, {keyA, false}, {keyRtShift, false}}, "A", ""},
		{"inert keys", slices.Concat(hold(keyLeftShift, press(keyPageUp)), hold(keyLeftCtrl, hold(keyLeftAlt, press(keyDelete)))), "", ""},
	}
	for _, csiMods := range []bool{false, true} {
		km := testKeymap(t)
		km.csiMods = csiMods
		for _, c := range cases {
			want := c.want
			if csiMods && c.csiMods != "" {
				want = c.csiMods
			}
			k := &linuxKeys{km: km}
			got, _ := typedBy(k, c.strokes)
			if got != want {
				t.Errorf("%s (csiMods %v): typed %q, want %q", c.name, csiMods, got, want)
			}
			if k.shift != 0 {
				t.Errorf("%s (csiMods %v): modifiers %#x left down", c.name, csiMods, k.shift)
			}
		}
	}
}

// Keys that switch consoles, the locks and Scroll Lock ask the console for
// what the kernel would do cooked: the locks' flags, a console by number or
// the one before or after, and output going on
func TestLinuxConsoleKeysAskTheConsole(t *testing.T) {
	cases := []struct {
		name    string
		strokes []stroke
		want    []linuxAct
		leds    uint8
	}{
		{"alt f2", hold(keyLeftAlt, press(keyF2)), []linuxAct{{vt: 2}}, 0},
		{"ctrl alt f1", hold(keyLeftCtrl, hold(keyLeftAlt, press(keyF1))), []linuxAct{{vt: 1}}, 0},
		{"altgr f1", hold(keyRightAlt, press(keyF1)), []linuxAct{{vt: 13}}, 0},
		{"alt left right", hold(keyLeftAlt, press(keyLeft, keyRight)), []linuxAct{{vt: vtPrev}, {vt: vtNext}}, 0},
		{"caps lock twice", press(keyCapsLock, keyCapsLock), []linuxAct{{leds: true}, {leds: true}}, 0},
		{"caps lock held", []stroke{{keyCapsLock, true}, {keyCapsLock, true}, {keyCapsLock, false}}, []linuxAct{{leds: true}}, ledCaps},
		{"num lock", press(keyNumLock), []linuxAct{{leds: true}}, ledNum},
		{"scroll lock", press(keyScrollLk), []linuxAct{{start: true}}, 0},
	}
	for _, c := range cases {
		k := &linuxKeys{km: testKeymap(t)}
		if _, got := typedBy(k, c.strokes); !slices.Equal(got, c.want) {
			t.Errorf("%s: asked %+v, want %+v", c.name, got, c.want)
		}
		if k.leds != c.leds {
			t.Errorf("%s: lock flags %#x, want %#x", c.name, k.leds, c.leds)
		}
	}
}

// Translation starts from the console's state: modifiers already down and the
// locks already on
func TestLinuxConsoleSeedsFromTheConsole(t *testing.T) {
	k := &linuxKeys{km: testKeymap(t)}
	k.seed(1<<kgShift, ledNum)
	got, _ := typedBy(k, slices.Concat(press(keyA, keyKP8), []stroke{{keyLeftShift, false}}, press(keyA)))
	if got != "A8a" {
		t.Errorf("typed %q, want %q", got, "A8a")
	}
}

// Sticky and locked modifiers, which some keymaps bind, hold for the next key
// and until locked off; a key no map has types nothing
func TestLinuxConsoleLocks(t *testing.T) {
	km := testKeymap(t)
	const keyShiftLock, keyStickyShift, keyNoMap = 85, 86, 89
	km.maps[0][keyShiftLock] = 0xf000 | ktLock<<8 | kgShift
	km.maps[1][keyShiftLock] = 0xf000 | ktLock<<8 | kgShift
	km.maps[0][keyStickyShift] = 0xf000 | ktSLock<<8 | kgShift
	km.maps[1][keyStickyShift] = 0xf000 | ktSLock<<8 | kgShift
	k := &linuxKeys{km: km}
	strokes := slices.Concat(press(keyShiftLock), press(keyA, keyA), press(keyShiftLock), press(keyStickyShift), press(keyA, keyA))
	if got, _ := typedBy(k, strokes); got != "AAAa" {
		t.Errorf("typed %q, want %q", got, "AAAa")
	}
	if got, _ := typedBy(k, hold(keyLeftAlt, hold(keyLeftShift, press(keyNoMap)))); got != "" {
		t.Errorf("a key with no map typed %q", got)
	}
}

// Dead keys and compose type what the console's accent table makes of the
// character after them, as cooked: their accented character, the dead key's
// own for a space or itself, or the dead key's character and the next when
// the table has no pair for them. A key typing no character leaves a dead key
// pending, and Enter types it.
func TestLinuxConsoleDeadKeys(t *testing.T) {
	km := testKeymap(t)
	km.maps[0][keyGrave] = 0xf000 | ktDead<<8        // dead grave, by number
	km.maps[1][keyGrave] = 0xf000 | ktDead2<<8 | '~' // dead tilde, by its character
	km.maps[0][keyCompose] = 0xf000 | ktSpec<<8 | 14 // K_COMPOSE
	km.accents = []linuxAccent{{'`', 'a', 'à'}, {'~', 'n', 'ñ'}, {'o', 'a', 'å'}, {'s', 's', 'ß'}}
	cases := []struct {
		name    string
		strokes []stroke
		want    string
	}{
		{"dead key", press(keyGrave, keyA), "à"},
		{"dead key by its character", slices.Concat(hold(keyLeftShift, press(keyGrave)), press(keyN)), "ñ"},
		{"dead key and space", press(keyGrave, keySpace), "`"},
		{"dead key twice", press(keyGrave, keyGrave, keyA), "à"},
		{"no pair", press(keyGrave, keyB), "`b"},
		{"dead key and enter", press(keyGrave, keyEnter), "`\r"},
		{"over a cursor key", press(keyGrave, keyUp, keyA), "\x1b[Aà"},
		{"compose", press(keyCompose, keyO, keyA), "å"},
		{"compose a pair", press(keyCompose, keyS, keyS), "ß"},
		{"compose and a dead key", press(keyCompose, keyGrave, keyA, keyB), "àb"},
	}
	for _, c := range cases {
		k := &linuxKeys{km: km}
		if got, _ := typedBy(k, c.strokes); got != c.want {
			t.Errorf("%s: typed %q, want %q", c.name, got, c.want)
		}
	}
}

// K_MEDIUMRAW input decodes to keycodes, a long one across reads, and a long
// one broken off is dropped without the key after it
func TestMediumRawDecodes(t *testing.T) {
	type ev struct {
		code int
		down bool
	}
	var got []ev
	var m mediumRaw
	key := func(code int, down bool) { got = append(got, ev{code, down}) }
	m.decode([]byte{30, 30 | 0x80, 0x00, 0x81}, key)
	m.decode([]byte{0x80 | 55, 0x80, 0x82, 0x80 | 3}, key)
	m.decode([]byte{0x00, 0x81, 28, 0x80 | 28}, key)
	want := []ev{{30, true}, {30, false}, {1<<7 | 55, true}, {2<<7 | 3, false}, {28, true}, {28, false}}
	if !slices.Equal(got, want) {
		t.Errorf("decoded %v, want %v", got, want)
	}
}
