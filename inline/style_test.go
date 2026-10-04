package inline

import (
	"testing"

	"github.com/lixenwraith/color"
)

// ANSI indices go out as 30-37/90-97 in every mode, so a text console and a
// themed emulator both render them; RGB degrades to them only in 16 colors.
func TestPaintEmitsANSIIndicesInEveryMode(t *testing.T) {
	cases := []struct {
		mode colorMode
		st   Style
		want string
	}{
		{colorModeTrueColor, FgANSI(color.ANSIRed), "\x1b[0;31mx\x1b[0m"},
		{colorMode256, FgANSI(color.ANSIBrightCyan).BgANSI(color.ANSIBlue), "\x1b[0;96;44mx\x1b[0m"},
		{colorMode16, FgANSI(color.ANSIBrightBlack).Bold(), "\x1b[0;1;90mx\x1b[0m"},
		{colorMode16, Fg(color.Red).Bg(color.Black), "\x1b[0;91;40mx\x1b[0m"},
		{colorMode256, Fg(color.Red), "\x1b[0;38;5;196mx\x1b[0m"},
		{colorModeTrueColor, Fg(color.RGB{R: 1, G: 2, B: 3}), "\x1b[0;38;2;1;2;3mx\x1b[0m"},
	}
	for _, c := range cases {
		p := &Printer{color: true, mode: c.mode}
		if got := p.Paint("x", c.st); got != c.want {
			t.Errorf("mode %d: %q, want %q", c.mode, got, c.want)
		}
	}
}

// A text console gets 16 colors even when a profile exported COLORTERM
func TestTextConsoleDetectsSixteenColors(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	for term, want := range map[string]colorMode{
		"linux": colorMode16, "cons25": colorMode16, "vt220": colorMode16,
		"xterm-16color": colorMode16, "xterm-256color": colorModeTrueColor,
	} {
		t.Setenv("TERM", term)
		if got := detectColorMode(); got != want {
			t.Errorf("TERM=%s: mode %d, want %d", term, got, want)
		}
	}
}
