package terminal

// Key represents a parsed input key
type Key uint16

// Key constants - designed for expansion
const (
	KeyNone Key = iota
	KeyRune     // Printable character (check Event.Rune)

	// Control keys
	KeyEscape
	KeyEnter
	KeyTab
	KeyBacktab  // Shift+Tab
	KeyShiftTab // Same as KeyBacktab,for clarity
	KeyBackspace
	KeyDelete
	KeySpace

	// Navigation
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert

	// Function keys
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12

	// Ctrl+letter (Ctrl+A = 0x01, Ctrl+Z = 0x1A)
	KeyCtrlA
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlF
	KeyCtrlG
	KeyCtrlH // Often same as Backspace
	KeyCtrlI // Often same as Tab
	KeyCtrlJ // Often same as Enter
	KeyCtrlK
	KeyCtrlL
	KeyCtrlM // Often same as Enter
	KeyCtrlN
	KeyCtrlO
	KeyCtrlP
	KeyCtrlQ
	KeyCtrlR
	KeyCtrlS
	KeyCtrlT
	KeyCtrlU
	KeyCtrlV
	KeyCtrlW
	KeyCtrlX
	KeyCtrlY
	KeyCtrlZ

	// Ctrl+special
	KeyCtrlSpace // Ctrl+@ / Ctrl+Space (Ctrl+@ produces NUL byte 0x00)
	KeyCtrlBackslash
	KeyCtrlBracketLeft
	KeyCtrlBracketRight
	KeyCtrlCaret
	KeyCtrlUnderscore
)

// Modifier flags
type Modifier uint8

const (
	ModNone  Modifier = 0
	ModShift Modifier = 1 << 0
	ModAlt   Modifier = 1 << 1
	ModCtrl  Modifier = 1 << 2
)

// escapeSequence maps escape sequences to keys
// Key: sequence after ESC [ (e.g., "A" for up arrow)
type escapeSequence struct {
	seq string
	key Key
	mod Modifier
}

// Known escape sequences (CSI sequences: ESC [ ...)
var csiSequences = []escapeSequence{
	// Arrow keys
	{"A", KeyUp, ModNone},
	{"B", KeyDown, ModNone},
	{"C", KeyRight, ModNone},
	{"D", KeyLeft, ModNone},
	{"Z", KeyBacktab, ModShift}, // Shift+Tab

	// Arrow keys with modifiers (xterm style: ESC [ 1 ; mod X)
	{"1;2A", KeyUp, ModShift},
	{"1;2B", KeyDown, ModShift},
	{"1;2C", KeyRight, ModShift},
	{"1;2D", KeyLeft, ModShift},
	{"1;3A", KeyUp, ModAlt},
	{"1;3B", KeyDown, ModAlt},
	{"1;3C", KeyRight, ModAlt},
	{"1;3D", KeyLeft, ModAlt},
	{"1;5A", KeyUp, ModCtrl},
	{"1;5B", KeyDown, ModCtrl},
	{"1;5C", KeyRight, ModCtrl},
	{"1;5D", KeyLeft, ModCtrl},

	// Navigation
	{"H", KeyHome, ModNone},
	{"F", KeyEnd, ModNone},
	{"1~", KeyHome, ModNone},
	{"4~", KeyEnd, ModNone},
	{"5~", KeyPageUp, ModNone},
	{"6~", KeyPageDown, ModNone},
	{"2~", KeyInsert, ModNone},
	{"3~", KeyDelete, ModNone},
	{"7~", KeyHome, ModNone},
	{"8~", KeyEnd, ModNone},

	// Function keys (xterm)
	{"11~", KeyF1, ModNone},
	{"12~", KeyF2, ModNone},
	{"13~", KeyF3, ModNone},
	{"14~", KeyF4, ModNone},
	{"15~", KeyF5, ModNone},
	{"17~", KeyF6, ModNone},
	{"18~", KeyF7, ModNone},
	{"19~", KeyF8, ModNone},
	{"20~", KeyF9, ModNone},
	{"21~", KeyF10, ModNone},
	{"23~", KeyF11, ModNone},
	{"24~", KeyF12, ModNone},

	// Function keys (vt style)
	{"[A", KeyF1, ModNone},
	{"[B", KeyF2, ModNone},
	{"[C", KeyF3, ModNone},
	{"[D", KeyF4, ModNone},
	{"[E", KeyF5, ModNone},

	// Shift+Navigation (mod=2)
	{"1;2H", KeyHome, ModShift},
	{"1;2F", KeyEnd, ModShift},
	{"2;2~", KeyInsert, ModShift},
	{"3;2~", KeyDelete, ModShift},
	{"5;2~", KeyPageUp, ModShift},
	{"6;2~", KeyPageDown, ModShift},

	// Alt+Arrows (mod=3) - already have 1;3A-D
	// Alt+Navigation (mod=3)
	{"1;3H", KeyHome, ModAlt},
	{"1;3F", KeyEnd, ModAlt},
	{"2;3~", KeyInsert, ModAlt},
	{"3;3~", KeyDelete, ModAlt},
	{"5;3~", KeyPageUp, ModAlt},
	{"6;3~", KeyPageDown, ModAlt},

	// Shift+Alt (mod=4)
	{"1;4A", KeyUp, ModShift | ModAlt},
	{"1;4B", KeyDown, ModShift | ModAlt},
	{"1;4C", KeyRight, ModShift | ModAlt},
	{"1;4D", KeyLeft, ModShift | ModAlt},
	{"1;4H", KeyHome, ModShift | ModAlt},
	{"1;4F", KeyEnd, ModShift | ModAlt},
	{"2;4~", KeyInsert, ModShift | ModAlt},
	{"3;4~", KeyDelete, ModShift | ModAlt},
	{"5;4~", KeyPageUp, ModShift | ModAlt},
	{"6;4~", KeyPageDown, ModShift | ModAlt},

	// Ctrl+Arrows (mod=5) - already have 1;5A-D
	// Ctrl+Navigation (mod=5)
	{"1;5H", KeyHome, ModCtrl},
	{"1;5F", KeyEnd, ModCtrl},
	{"2;5~", KeyInsert, ModCtrl},
	{"3;5~", KeyDelete, ModCtrl},
	{"5;5~", KeyPageUp, ModCtrl},
	{"6;5~", KeyPageDown, ModCtrl},

	// Shift+Ctrl (mod=6)
	{"1;6A", KeyUp, ModShift | ModCtrl},
	{"1;6B", KeyDown, ModShift | ModCtrl},
	{"1;6C", KeyRight, ModShift | ModCtrl},
	{"1;6D", KeyLeft, ModShift | ModCtrl},
	{"1;6H", KeyHome, ModShift | ModCtrl},
	{"1;6F", KeyEnd, ModShift | ModCtrl},
	{"2;6~", KeyInsert, ModShift | ModCtrl},
	{"3;6~", KeyDelete, ModShift | ModCtrl},
	{"5;6~", KeyPageUp, ModShift | ModCtrl},
	{"6;6~", KeyPageDown, ModShift | ModCtrl},

	// Alt+Ctrl (mod=7)
	{"1;7A", KeyUp, ModAlt | ModCtrl},
	{"1;7B", KeyDown, ModAlt | ModCtrl},
	{"1;7C", KeyRight, ModAlt | ModCtrl},
	{"1;7D", KeyLeft, ModAlt | ModCtrl},
	{"1;7H", KeyHome, ModAlt | ModCtrl},
	{"1;7F", KeyEnd, ModAlt | ModCtrl},
	{"2;7~", KeyInsert, ModAlt | ModCtrl},
	{"3;7~", KeyDelete, ModAlt | ModCtrl},
	{"5;7~", KeyPageUp, ModAlt | ModCtrl},
	{"6;7~", KeyPageDown, ModAlt | ModCtrl},

	// Shift+Alt+Ctrl (mod=8)
	{"1;8A", KeyUp, ModShift | ModAlt | ModCtrl},
	{"1;8B", KeyDown, ModShift | ModAlt | ModCtrl},
	{"1;8C", KeyRight, ModShift | ModAlt | ModCtrl},
	{"1;8D", KeyLeft, ModShift | ModAlt | ModCtrl},
	{"1;8H", KeyHome, ModShift | ModAlt | ModCtrl},
	{"1;8F", KeyEnd, ModShift | ModAlt | ModCtrl},
	{"2;8~", KeyInsert, ModShift | ModAlt | ModCtrl},
	{"3;8~", KeyDelete, ModShift | ModAlt | ModCtrl},
	{"5;8~", KeyPageUp, ModShift | ModAlt | ModCtrl},
	{"6;8~", KeyPageDown, ModShift | ModAlt | ModCtrl},

	// F-keys with modifiers (CSI style: ESC [ 1 ; mod P/Q/R/S for F1-F4)
	// F1-F4 with Shift (mod=2)
	{"1;2P", KeyF1, ModShift},
	{"1;2Q", KeyF2, ModShift},
	{"1;2R", KeyF3, ModShift},
	{"1;2S", KeyF4, ModShift},
	// F1-F4 with Alt (mod=3)
	{"1;3P", KeyF1, ModAlt},
	{"1;3Q", KeyF2, ModAlt},
	{"1;3R", KeyF3, ModAlt},
	{"1;3S", KeyF4, ModAlt},
	// F1-F4 with Ctrl (mod=5)
	{"1;5P", KeyF1, ModCtrl},
	{"1;5Q", KeyF2, ModCtrl},
	{"1;5R", KeyF3, ModCtrl},
	{"1;5S", KeyF4, ModCtrl},
	// F1-F4 with Shift+Alt (mod=4)
	{"1;4P", KeyF1, ModShift | ModAlt},
	{"1;4Q", KeyF2, ModShift | ModAlt},
	{"1;4R", KeyF3, ModShift | ModAlt},
	{"1;4S", KeyF4, ModShift | ModAlt},
	// F1-F4 with Shift+Ctrl (mod=6)
	{"1;6P", KeyF1, ModShift | ModCtrl},
	{"1;6Q", KeyF2, ModShift | ModCtrl},
	{"1;6R", KeyF3, ModShift | ModCtrl},
	{"1;6S", KeyF4, ModShift | ModCtrl},
	// F1-F4 with Alt+Ctrl (mod=7)
	{"1;7P", KeyF1, ModAlt | ModCtrl},
	{"1;7Q", KeyF2, ModAlt | ModCtrl},
	{"1;7R", KeyF3, ModAlt | ModCtrl},
	{"1;7S", KeyF4, ModAlt | ModCtrl},
	// F1-F4 with Shift+Alt+Ctrl (mod=8)
	{"1;8P", KeyF1, ModShift | ModAlt | ModCtrl},
	{"1;8Q", KeyF2, ModShift | ModAlt | ModCtrl},
	{"1;8R", KeyF3, ModShift | ModAlt | ModCtrl},
	{"1;8S", KeyF4, ModShift | ModAlt | ModCtrl},

	// F5-F12 with modifiers (ESC [ N ; mod ~)
	// F5 (15~)
	{"15;2~", KeyF5, ModShift},
	{"15;3~", KeyF5, ModAlt},
	{"15;4~", KeyF5, ModShift | ModAlt},
	{"15;5~", KeyF5, ModCtrl},
	{"15;6~", KeyF5, ModShift | ModCtrl},
	{"15;7~", KeyF5, ModAlt | ModCtrl},
	{"15;8~", KeyF5, ModShift | ModAlt | ModCtrl},
	// F6 (17~)
	{"17;2~", KeyF6, ModShift},
	{"17;3~", KeyF6, ModAlt},
	{"17;4~", KeyF6, ModShift | ModAlt},
	{"17;5~", KeyF6, ModCtrl},
	{"17;6~", KeyF6, ModShift | ModCtrl},
	{"17;7~", KeyF6, ModAlt | ModCtrl},
	{"17;8~", KeyF6, ModShift | ModAlt | ModCtrl},
	// F7 (18~)
	{"18;2~", KeyF7, ModShift},
	{"18;3~", KeyF7, ModAlt},
	{"18;4~", KeyF7, ModShift | ModAlt},
	{"18;5~", KeyF7, ModCtrl},
	{"18;6~", KeyF7, ModShift | ModCtrl},
	{"18;7~", KeyF7, ModAlt | ModCtrl},
	{"18;8~", KeyF7, ModShift | ModAlt | ModCtrl},
	// F8 (19~)
	{"19;2~", KeyF8, ModShift},
	{"19;3~", KeyF8, ModAlt},
	{"19;4~", KeyF8, ModShift | ModAlt},
	{"19;5~", KeyF8, ModCtrl},
	{"19;6~", KeyF8, ModShift | ModCtrl},
	{"19;7~", KeyF8, ModAlt | ModCtrl},
	{"19;8~", KeyF8, ModShift | ModAlt | ModCtrl},
	// F9 (20~)
	{"20;2~", KeyF9, ModShift},
	{"20;3~", KeyF9, ModAlt},
	{"20;4~", KeyF9, ModShift | ModAlt},
	{"20;5~", KeyF9, ModCtrl},
	{"20;6~", KeyF9, ModShift | ModCtrl},
	{"20;7~", KeyF9, ModAlt | ModCtrl},
	{"20;8~", KeyF9, ModShift | ModAlt | ModCtrl},
	// F10 (21~)
	{"21;2~", KeyF10, ModShift},
	{"21;3~", KeyF10, ModAlt},
	{"21;4~", KeyF10, ModShift | ModAlt},
	{"21;5~", KeyF10, ModCtrl},
	{"21;6~", KeyF10, ModShift | ModCtrl},
	{"21;7~", KeyF10, ModAlt | ModCtrl},
	{"21;8~", KeyF10, ModShift | ModAlt | ModCtrl},
	// F11 (23~)
	{"23;2~", KeyF11, ModShift},
	{"23;3~", KeyF11, ModAlt},
	{"23;4~", KeyF11, ModShift | ModAlt},
	{"23;5~", KeyF11, ModCtrl},
	{"23;6~", KeyF11, ModShift | ModCtrl},
	{"23;7~", KeyF11, ModAlt | ModCtrl},
	{"23;8~", KeyF11, ModShift | ModAlt | ModCtrl},
	// F12 (24~)
	{"24;2~", KeyF12, ModShift},
	{"24;3~", KeyF12, ModAlt},
	{"24;4~", KeyF12, ModShift | ModAlt},
	{"24;5~", KeyF12, ModCtrl},
	{"24;6~", KeyF12, ModShift | ModCtrl},
	{"24;7~", KeyF12, ModAlt | ModCtrl},
	{"24;8~", KeyF12, ModShift | ModAlt | ModCtrl},
}

// SS3 sequences (ESC O ...)
var ss3Sequences = []escapeSequence{
	{"A", KeyUp, ModNone},
	{"B", KeyDown, ModNone},
	{"C", KeyRight, ModNone},
	{"D", KeyLeft, ModNone},
	{"H", KeyHome, ModNone},
	{"F", KeyEnd, ModNone},
	{"P", KeyF1, ModNone},
	{"Q", KeyF2, ModNone},
	{"R", KeyF3, ModNone},
	{"S", KeyF4, ModNone},

	// Numeric Keypad (Application Mode)
	{"M", KeyEnter, ModNone}, // Keypad Enter
	{"X", KeyRune, ModNone},  // Keypad = (some terminals)
	{"j", KeyRune, ModNone},  // Keypad *
	{"k", KeyRune, ModNone},  // Keypad +
	{"l", KeyRune, ModNone},  // Keypad ,
	{"m", KeyRune, ModNone},  // Keypad -
	{"n", KeyRune, ModNone},  // Keypad .
	{"o", KeyRune, ModNone},  // Keypad /
	{"p", KeyRune, ModNone},  // Keypad 0
	{"q", KeyRune, ModNone},  // Keypad 1
	{"r", KeyRune, ModNone},  // Keypad 2
	{"s", KeyRune, ModNone},  // Keypad 3
	{"t", KeyRune, ModNone},  // Keypad 4
	{"u", KeyRune, ModNone},  // Keypad 5
	{"v", KeyRune, ModNone},  // Keypad 6
	{"w", KeyRune, ModNone},  // Keypad 7
	{"x", KeyRune, ModNone},  // Keypad 8
	{"y", KeyRune, ModNone},  // Keypad 9
}

var csiMap = buildSequenceMap(csiSequences)
var ss3Map = buildSequenceMap(ss3Sequences)

func buildSequenceMap(seqs []escapeSequence) map[string]escapeSequence {
	m := make(map[string]escapeSequence, len(seqs))
	for _, s := range seqs {
		m[s.seq] = s
	}
	return m
}

// lookupCSI performs zero-alloc map lookup via compiler optimization
// The string([]byte) conversion inline in map access does not allocate
func lookupCSI(seq []byte) (Key, Modifier, bool) {
	if s, ok := csiMap[string(seq)]; ok {
		return s.key, s.mod, true
	}
	return KeyNone, ModNone, false
}

// lookupSS3 performs zero-alloc map lookup
func lookupSS3(seq []byte) (Key, Modifier, bool) {
	if s, ok := ss3Map[string(seq)]; ok {
		return s.key, s.mod, true
	}
	return KeyNone, ModNone, false
}