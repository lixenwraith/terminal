//go:build !unix && !windows

package inline

import "os"

func detectColorMode() colorMode {
	return colorMode256
}

func windowSize(f *os.File) (w, h int, ok bool) {
	return 0, 0, false
}
