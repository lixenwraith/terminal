//go:build unix

package terminal

import (
	"os"

	"golang.org/x/sys/unix"
)

// controllingTTY opens the controlling terminal for writing, nil without one
func controllingTTY() *os.File {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return nil
	}
	return f
}

// resetTerminalMode attempts to restore terminal to cooked mode
// Best-effort for crash recovery; errors ignored
func resetTerminalMode() {
	// Try to restore via /dev/tty (works even if stdin redirected)
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		fd := int(tty.Fd())
		// Get current termios, enable ECHO and ICANON
		if termios, err := unix.IoctlGetTermios(fd, tcgets); err == nil {
			termios.Lflag |= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
			termios.Iflag |= unix.ICRNL
			unix.IoctlSetTermios(fd, tcsets, termios)
		}
	}
}
