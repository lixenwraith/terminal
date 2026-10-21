//go:build unix && !linux

package terminal

import (
	"os"

	"golang.org/x/sys/unix"
)

// openConsole finds no console whose keyboard goes raw here
func openConsole(*os.File, *unix.Termios) keyConsole { return nil }
