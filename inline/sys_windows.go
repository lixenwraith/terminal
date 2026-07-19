//go:build windows

package inline

import (
	"os"

	"golang.org/x/sys/windows"
)

func detectColorMode() colorMode {
	if os.Getenv("WT_SESSION") != "" || os.Getenv("WT_PROFILE_ID") != "" {
		return colorModeTrueColor
	}
	if ct := os.Getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		return colorModeTrueColor
	}
	return colorMode256
}

func windowSize(f *os.File) (w, h int, ok bool) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(f.Fd()), &info); err != nil {
		return 0, 0, false
	}
	width := int(info.Window.Right-info.Window.Left) + 1
	height := int(info.Window.Bottom-info.Window.Top) + 1
	if width < 1 || height < 1 {
		return 0, 0, false
	}
	return width, height, true
}
