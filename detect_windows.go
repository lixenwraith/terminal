//go:build windows

package terminal

import (
	"os"

	"github.com/lixenwraith/terminal/internal/env"
	"golang.org/x/sys/windows"
)

// trueColorBuild is the first Windows build whose console draws 24-bit color, 10 1703
const trueColorBuild = 15063

// DetectColorMode reads the environment as on Unix where it names the terminal
// (TERM or COLORTERM set, as mintty, ssh or Alacritty do). A native console names
// none, and draws 24-bit color from Windows 10 1703 on, so it gets true color.
func DetectColorMode() ColorMode {
	named := os.Getenv("TERM") != "" || os.Getenv("COLORTERM") != ""
	return consoleColorMode(env.Colors(), named, windows.RtlGetVersion().BuildNumber)
}

// consoleColorMode chooses from what the environment says (env.Colors), whether
// it names the terminal, and the Windows build
func consoleColorMode(colors int, named bool, build uint32) ColorMode {
	switch {
	case colors == 16:
		return ColorMode16
	case colors == 1<<24, !named && build >= trueColorBuild:
		return ColorModeTrueColor
	}
	return ColorMode256
}

// consoleShown reports whether a std handle is on the console, which shows the
// process's console is the one the player sees, not a hidden one
func consoleShown() bool {
	var mode uint32
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil {
			return true
		}
	}
	return false
}

// controllingTTY opens the console for writing, nil without one the player sees
func controllingTTY() *os.File {
	if !consoleShown() {
		return nil
	}
	h, err := openConsoleDev("CONOUT$")
	if err != nil {
		return nil
	}
	return os.NewFile(uintptr(h), "CONOUT$")
}

func resetTerminalMode() {
	saneIn := uint32(windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_LINE_INPUT |
		windows.ENABLE_ECHO_INPUT |
		windows.ENABLE_MOUSE_INPUT |
		windows.ENABLE_QUICK_EDIT_MODE |
		windows.ENABLE_EXTENDED_FLAGS)
	saneOut := uint32(windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_WRAP_AT_EOL_OUTPUT)

	if h, err := openConsoleDev("CONIN$"); err == nil {
		windows.SetConsoleMode(h, saneIn)
		windows.CloseHandle(h)
	}
	if h, err := openConsoleDev("CONOUT$"); err == nil {
		windows.SetConsoleMode(h, saneOut)
		windows.CloseHandle(h)
	}
}

func openConsoleDev(name string) (windows.Handle, error) {
	return windows.CreateFile(
		windows.StringToUTF16Ptr(name),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
}
