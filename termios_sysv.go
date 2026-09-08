//go:build aix || linux || solaris || zos

package terminal

import "golang.org/x/sys/unix"

const (
	tcgets = unix.TCGETS
	tcsets = unix.TCSETS
)
