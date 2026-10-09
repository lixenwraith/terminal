// Package env reads what the environment says about a terminal's colors,
// for terminal and inline alike
package env

import (
	"os"
	"strings"
)

// Colors reports how many colors the environment says the terminal shows:
// 16 on a text console, which wins over COLORTERM since profiles export it
// everywhere; 1<<24 when the terminal announces true color; else 256
func Colors() int {
	switch {
	case textConsole(os.Getenv("TERM")):
		return 16
	case trueColor():
		return 1 << 24
	}
	return 256
}

// textConsole reports a TERM limited to 16 colors: the Linux VT, FreeBSD
// syscons, VT100-class terminals (vt and a digit, not vte) and *-16color
func textConsole(term string) bool {
	return term == "linux" || term == "ansi" || strings.HasPrefix(term, "cons25") ||
		len(term) > 2 && term[:2] == "vt" && term[2] >= '0' && term[2] <= '9' ||
		strings.HasSuffix(term, "-16color")
}

// trueColorVars are set by emulators that draw 24-bit color
var trueColorVars = []string{"KITTY_WINDOW_ID", "KONSOLE_VERSION", "ITERM_SESSION_ID",
	"ALACRITTY_WINDOW_ID", "ALACRITTY_LOG", "WEZTERM_PANE", "WT_SESSION", "WT_PROFILE_ID"}

func trueColor() bool {
	if ct := os.Getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		return true
	}
	for _, v := range trueColorVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	term := os.Getenv("TERM")
	return strings.Contains(term, "truecolor") || strings.Contains(term, "24bit") || strings.Contains(term, "direct")
}
