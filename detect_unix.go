//go:build unix

package terminal

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// DetectColorMode determines terminal color capability from environment
func DetectColorMode() ColorMode {
	colorterm := os.Getenv("COLORTERM")
	if colorterm == "truecolor" || colorterm == "24bit" {
		return ColorModeTrueColor
	}

	if os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("KONSOLE_VERSION") != "" ||
		os.Getenv("ITERM_SESSION_ID") != "" ||
		os.Getenv("ALACRITTY_WINDOW_ID") != "" ||
		os.Getenv("ALACRITTY_LOG") != "" ||
		os.Getenv("WEZTERM_PANE") != "" {
		return ColorModeTrueColor
	}

	term := os.Getenv("TERM")
	if strings.Contains(term, "truecolor") ||
		strings.Contains(term, "24bit") ||
		strings.Contains(term, "direct") {
		return ColorModeTrueColor
	}

	return ColorMode256
}

// resetTerminalMode attempts to restore terminal to cooked mode
// Best-effort for crash recovery; errors ignored
func resetTerminalMode() {
	// Try to restore via /dev/tty (works even if stdin redirected)
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		fd := int(tty.Fd())
		// Get current termios, enable ECHO and ICANON
		if termios, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
			termios.Lflag |= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
			termios.Iflag |= unix.ICRNL
			unix.IoctlSetTermios(fd, unix.TCSETS, termios)
		}
	}
}