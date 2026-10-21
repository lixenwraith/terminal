//go:build linux || freebsd

package terminal

import (
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// What the consoles whose keyboard goes raw share: the Linux console and
// FreeBSD's vt(4)

// ioctlPtr is an ioctl whose argument is a pointer
func ioctlPtr(fd int, req uint, arg unsafe.Pointer) error {
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// consoleRestore is what restores a console keyboard left raw
type consoleRestore struct {
	fd, mode int
	guard    *keyGuard
}

// rawConsoleKeys is the console a Terminal has raw, for EmergencyReset
var rawConsoleKeys atomic.Pointer[consoleRestore]

// restoreConsoleKeys switches back a console keyboard a Terminal has raw. It
// takes no lock, for a reset from a panic that may hold one.
func restoreConsoleKeys() {
	if r := rawConsoleKeys.Swap(nil); r != nil && unix.IoctlSetInt(r.fd, kdSetKeyboardMode, r.mode) == nil {
		flushInput(r.fd)
		r.guard.disarm()
	}
}
