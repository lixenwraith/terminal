//go:build unix

package inline

import (
	"os"

	"golang.org/x/sys/unix"
)

func windowSize(f *os.File) (w, h int, ok bool) {
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 0, 0, false
	}
	return int(ws.Col), int(ws.Row), true
}
