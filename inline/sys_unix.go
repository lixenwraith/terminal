//go:build unix

package inline

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func detectColorMode() colorMode {
	colorterm := os.Getenv("COLORTERM")
	if colorterm == "truecolor" || colorterm == "24bit" {
		return colorModeTrueColor
	}

	if os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("KONSOLE_VERSION") != "" ||
		os.Getenv("ITERM_SESSION_ID") != "" ||
		os.Getenv("ALACRITTY_WINDOW_ID") != "" ||
		os.Getenv("ALACRITTY_LOG") != "" ||
		os.Getenv("WEZTERM_PANE") != "" {
		return colorModeTrueColor
	}

	term := os.Getenv("TERM")
	if strings.Contains(term, "truecolor") ||
		strings.Contains(term, "24bit") ||
		strings.Contains(term, "direct") {
		return colorModeTrueColor
	}

	return colorMode256
}

func windowSize(f *os.File) (w, h int, ok bool) {
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 0, 0, false
	}
	return int(ws.Col), int(ws.Row), true
}
