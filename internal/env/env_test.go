package env

import "testing"

// A text console, VT100-class included, gets 16 colors whatever COLORTERM
// says; vte, a modern emulator, is not one
func TestTextConsoleWinsOverColorterm(t *testing.T) {
	for _, v := range trueColorVars {
		t.Setenv(v, "")
	}
	t.Setenv("COLORTERM", "truecolor")
	for term, want := range map[string]int{
		"linux": 16, "cons25": 16, "vt220": 16, "ansi": 16, "xterm-16color": 16,
		"vte-256color": 1 << 24, "vt": 1 << 24, "xterm-256color": 1 << 24,
	} {
		t.Setenv("TERM", term)
		if got := Colors(); got != want {
			t.Errorf("TERM=%s: %d colors, want %d", term, got, want)
		}
	}
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "vte-256color")
	if got := Colors(); got != 256 {
		t.Errorf("vte-256color: %d", got)
	}
}
