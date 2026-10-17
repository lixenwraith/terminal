package terminal

import "testing"

func TestControlKey(t *testing.T) {
	cases := map[rune]Key{
		'a': KeyCtrlA, 'G': KeyCtrlG, 'z': KeyCtrlZ,
		'h': KeyBackspace, 'i': KeyTab, 'j': KeyEnter, 'm': KeyEnter,
		' ': KeyCtrlSpace, '@': KeyCtrlSpace, '2': KeyCtrlSpace,
		'[': KeyEscape, '3': KeyEscape,
		'\\': KeyCtrlBackslash, '4': KeyCtrlBackslash,
		']': KeyCtrlBracketRight, '5': KeyCtrlBracketRight,
		'^': KeyCtrlCaret, '6': KeyCtrlCaret,
		'_': KeyCtrlUnderscore, '-': KeyCtrlUnderscore, '/': KeyCtrlUnderscore, '7': KeyCtrlUnderscore,
	}
	for r, want := range cases {
		if got, ok := ControlKey(r); !ok || got != want {
			t.Errorf("ControlKey(%q) = %v, %v; want %v, true", r, got, ok, want)
		}
	}
	for _, r := range []rune{'1', '8', '.', '~', 'é', 0} {
		if _, ok := ControlKey(r); ok {
			t.Errorf("ControlKey(%q) ok, want none", r)
		}
	}
}
