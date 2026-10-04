//go:build !unix && !windows

package inline

import "os"

func detectColorMode() colorMode {
	if textConsole() {
		return colorMode16
	}
	return colorMode256
}

func windowSize(f *os.File) (w, h int, ok bool) {
	return 0, 0, false
}
