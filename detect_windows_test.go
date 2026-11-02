//go:build windows

package terminal

import "testing"

// A native console, which names no terminal, gets true color from Windows 10
// 1703 on; an environment that names the terminal is read as on Unix
func TestNativeConsoleIsTrueColor(t *testing.T) {
	for _, c := range []struct {
		colors int
		named  bool
		build  uint32
		want   ColorMode
	}{
		{256, false, 26100, ColorModeTrueColor}, // Windows 11 24H2, cmd or PowerShell
		{256, false, 14393, ColorMode256},       // Windows 10 1607
		{256, true, 26100, ColorMode256},        // TERM=xterm-256color, over ssh
		{1 << 24, true, 26100, ColorModeTrueColor},
		{16, true, 26100, ColorMode16},
	} {
		if got := consoleColorMode(c.colors, c.named, c.build); got != c.want {
			t.Errorf("%+v: mode %d", c, got)
		}
	}
}
