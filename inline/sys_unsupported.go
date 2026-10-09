//go:build !unix && !windows

package inline

import "os"

func windowSize(f *os.File) (w, h int, ok bool) {
	return 0, 0, false
}
