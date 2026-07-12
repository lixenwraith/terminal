package terminal

// keyToName maps Key constants to canonical config string names
var keyToName = map[Key]string{
	KeyEscape:    "escape",
	KeyEnter:     "enter",
	KeyTab:       "tab",
	KeyBacktab:   "backtab",
	KeyBackspace: "backspace",
	KeyDelete:    "delete",
	KeySpace:     "space",

	KeyUp:       "up",
	KeyDown:     "down",
	KeyLeft:     "left",
	KeyRight:    "right",
	KeyHome:     "home",
	KeyEnd:      "end",
	KeyPageUp:   "page_up",
	KeyPageDown: "page_down",
	KeyInsert:   "insert",

	KeyF1:  "f1",
	KeyF2:  "f2",
	KeyF3:  "f3",
	KeyF4:  "f4",
	KeyF5:  "f5",
	KeyF6:  "f6",
	KeyF7:  "f7",
	KeyF8:  "f8",
	KeyF9:  "f9",
	KeyF10: "f10",
	KeyF11: "f11",
	KeyF12: "f12",

	KeyCtrlA:            "ctrl_a",
	KeyCtrlB:            "ctrl_b",
	KeyCtrlC:            "ctrl_c",
	KeyCtrlD:            "ctrl_d",
	KeyCtrlE:            "ctrl_e",
	KeyCtrlF:            "ctrl_f",
	KeyCtrlG:            "ctrl_g",
	KeyCtrlH:            "ctrl_h",
	KeyCtrlI:            "ctrl_i",
	KeyCtrlJ:            "ctrl_j",
	KeyCtrlK:            "ctrl_k",
	KeyCtrlL:            "ctrl_l",
	KeyCtrlM:            "ctrl_m",
	KeyCtrlN:            "ctrl_n",
	KeyCtrlO:            "ctrl_o",
	KeyCtrlP:            "ctrl_p",
	KeyCtrlQ:            "ctrl_q",
	KeyCtrlR:            "ctrl_r",
	KeyCtrlS:            "ctrl_s",
	KeyCtrlT:            "ctrl_t",
	KeyCtrlU:            "ctrl_u",
	KeyCtrlV:            "ctrl_v",
	KeyCtrlW:            "ctrl_w",
	KeyCtrlX:            "ctrl_x",
	KeyCtrlY:            "ctrl_y",
	KeyCtrlZ:            "ctrl_z",
	KeyCtrlSpace:        "ctrl_space",
	KeyCtrlBackslash:    "ctrl_backslash",
	KeyCtrlBracketLeft:  "ctrl_bracket_left",
	KeyCtrlBracketRight: "ctrl_bracket_right",
	KeyCtrlCaret:        "ctrl_caret",
	KeyCtrlUnderscore:   "ctrl_underscore",
}

// nameToKey is the reverse lookup, built from keyToName
var nameToKey map[string]Key

func init() {
	nameToKey = make(map[string]Key, len(keyToName))
	for k, v := range keyToName {
		nameToKey[v] = k
	}
	// Aliases
	nameToKey["shift_tab"] = KeyBacktab
}

// KeyName returns the canonical string name for a Key constant
// Returns empty string for KeyNone and KeyRune
func KeyName(k Key) string {
	return keyToName[k]
}

// KeyByName resolves a canonical name to a Key constant
// Returns KeyNone and false if name is unknown
func KeyByName(name string) (Key, bool) {
	k, ok := nameToKey[name]
	return k, ok
}