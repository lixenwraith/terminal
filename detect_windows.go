//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

func DetectColorMode() ColorMode {
	if os.Getenv("WT_SESSION") != "" || os.Getenv("WT_PROFILE_ID") != "" {
		return ColorModeTrueColor
	}
	if ct := os.Getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		return ColorModeTrueColor
	}
	return ColorMode256
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
