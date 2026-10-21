//go:build freebsd

package terminal

import (
	"os"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The FreeBSD console's ioctls (sys/kbio.h, sys/consio.h), which
// golang.org/x/sys/unix does not define
const (
	kdGetKeyboardMode = 0x40044B06 // KDGKBMODE
	kdSetKeyboardMode = 0x20044B07 // KDSKBMODE
	kdGetLockState    = 0x40044B13 // KDGKBSTATE
	kdSetLockState    = 0x20044B14 // KDSKBSTATE
	gioKeymap         = 0x20006B06 // GIO_KEYMAP
	gioDeadKeymap     = 0x20006B08 // GIO_DEADKEYMAP
	vtActivate        = 0x20047605 // VT_ACTIVATE
	vtGetActive       = 0x40047607 // VT_GETACTIVE
	vtGetIndex        = 0x40047608 // VT_GETINDEX
)

// Keyboard modes
const (
	kXlate = 1 // K_XLATE
	kCode  = 2 // K_CODE
)

// keymapT is GIO_KEYMAP's keymap_t
type keymapT struct {
	nKeys uint16
	keys  [fbsdNumKeys]fbsdKey
}

// accentmapT is GIO_DEADKEYMAP's accentmap_t
type accentmapT struct {
	nAccs   uint16
	accents [fbsdDeadKeys]fbsdAccent
}

// freebsdConsole is the vt(4) console the game runs on, its keyboard switched
// to keycodes (K_CODE) while releases come from it
type freebsdConsole struct {
	tty     *os.File
	fd      int
	vt      int           // the console's number, from 1
	termios *unix.Termios // the termios the backend found, which the guard restores

	mu    sync.Mutex
	isRaw bool
	km    fbsdKeymap
	kb    fbsdKeys
	away  bool // another console had the keyboard when last asked
	guard *keyGuard
}

// openConsole is the console tty is, nil unless it is one: a vt(4) console,
// not a pty, a serial line or syscons
func openConsole(tty *os.File, cooked *unix.Termios) keyConsole {
	if vty, err := unix.Sysctl("kern.vty"); cooked == nil || err != nil || vty != "vt" {
		return nil
	}
	fd := int(tty.Fd())
	vt, err := unix.IoctlGetInt(fd, vtGetIndex)
	if err != nil || vt < 1 {
		return nil
	}
	if _, err := unix.IoctlGetInt(fd, kdGetKeyboardMode); err != nil {
		return nil
	}
	c := &freebsdConsole{tty: tty, fd: fd, vt: vt, termios: cooked}
	c.kb.km = &c.km
	return c
}

// prepare reads the keymap and starts the guard; a keyboard some other
// program has raw is left alone
func (c *freebsdConsole) prepare() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isRaw {
		return true
	}
	mode, err := unix.IoctlGetInt(c.fd, kdGetKeyboardMode)
	if err != nil || mode != kXlate || !c.readKeymap() {
		return false
	}
	if c.guard == nil {
		if c.guard, err = startGuard(c.tty); err != nil {
			return false
		}
	}
	return true
}

// readKeymap reads the keyboard's keymap and accents, and whether vt takes
// right Alt for AltGr
func (c *freebsdConsole) readKeymap() bool {
	var km keymapT
	if ioctlPtr(c.fd, gioKeymap, unsafe.Pointer(&km)) != nil {
		return false
	}
	c.km.keys = km.keys
	var am accentmapT
	c.km.nAccents = 0
	if ioctlPtr(c.fd, gioDeadKeymap, unsafe.Pointer(&am)) == nil {
		c.km.accents, c.km.nAccents = am.accents, min(int(am.nAccs), fbsdDeadKeys)
	}
	altGr, err := unix.SysctlUint32("kern.vt.enable_altgr")
	c.km.altGr = err != nil || altGr != 0
	return true
}

// raw arms the guard and switches the keyboard to keycodes, which vt allows
// whoever has the console open. Scroll Lock on is the console's history mode,
// which takes every key but its own: the keyboard stays as it is then.
func (c *freebsdConsole) raw() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isRaw {
		return true
	}
	if c.guard == nil || !c.guard.arm(kXlate, c.termios) {
		c.dropGuard()
		return false
	}
	locks, err := unix.IoctlGetInt(c.fd, kdGetLockState)
	if err != nil || locks&fbsdSLKED != 0 || unix.IoctlSetInt(c.fd, kdSetKeyboardMode, kCode) != nil {
		c.guard.disarm()
		return false
	}
	// What was typed cooked would read as keycodes
	flushInput(c.fd)
	c.kb.seed(uint32(locks))
	c.away, c.isRaw = false, true
	rawConsoleKeys.Store(&consoleRestore{fd: c.fd, mode: kXlate, guard: c.guard})
	return true
}

// cooked switches the keyboard back
func (c *freebsdConsole) cooked() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookedLocked()
}

// cookedLocked switches the keyboard back, dropping keycodes not yet read; a
// console that refuses keeps the guard armed. Caller MUST hold mu.
func (c *freebsdConsole) cookedLocked() {
	if !c.isRaw {
		return
	}
	c.isRaw = false
	rawConsoleKeys.Store(nil)
	if unix.IoctlSetInt(c.fd, kdSetKeyboardMode, kXlate) == nil {
		flushInput(c.fd)
		c.guard.disarm()
	}
}

// keys translates raw keycodes: one byte each, bit 7 set on release
func (c *freebsdConsole) keys(data []byte, key func(code uint32, action KeyAction, typed []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isRaw {
		return
	}
	for _, b := range data {
		code, down := int(b&0x7f), b&0x80 == 0
		if code == 0 {
			continue
		}
		action := KeyRelease
		if down {
			action = KeyPress
			if c.kb.isDown(code) {
				action = KeyRepeat
			}
		}
		typed, act := c.kb.key(code, down)
		c.act(act)
		key(uint32(code), action, typed)
	}
}

// act does what a key asks of the console: the locks it toggled kept with the
// keyboard and its lights, and a switch to another console. Caller MUST hold
// mu.
func (c *freebsdConsole) act(a consoleAct) {
	if a.leds {
		unix.IoctlSetInt(c.fd, kdSetLockState, int(c.kb.state&fbsdLockMask))
	}
	if a.vt != 0 {
		c.switchTo(a.vt)
	}
}

// switchTo switches to console vt, or the one before or after the one on
// screen, as vt does for a key cooked; vt itself refuses a console no one has
// open
func (c *freebsdConsole) switchTo(vt int) {
	cur, err := unix.IoctlGetInt(c.fd, vtGetActive)
	if err != nil {
		return
	}
	switch vt {
	case vtNext:
		vt = cur%fbsdWindows + 1
	case vtPrev:
		vt = (cur+fbsdWindows-2)%fbsdWindows + 1
	}
	if vt != cur {
		unix.IoctlSetInt(c.fd, vtActivate, vt)
	}
}

// front is whether this console is on screen; back on it, translation starts
// again from the console's locks, its modifiers having moved elsewhere
func (c *freebsdConsole) front() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	active, err := unix.IoctlGetInt(c.fd, vtGetActive)
	if !c.isRaw || err != nil {
		return true
	}
	front := active == c.vt
	if front && c.away {
		locks, _ := unix.IoctlGetInt(c.fd, kdGetLockState)
		c.kb.seed(uint32(locks))
	}
	c.away = !front
	return front
}

// close switches the keyboard back and ends the guard
func (c *freebsdConsole) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookedLocked()
	c.dropGuard()
}

// dropGuard ends the guard, which the next prepare starts again. Caller MUST
// hold mu.
func (c *freebsdConsole) dropGuard() {
	if c.guard != nil {
		c.guard.close()
		c.guard = nil
	}
}

// flushInput drops the console's pending input as its keyboard switches:
// keycodes would read as characters, and characters as keycodes
func flushInput(fd int) {
	unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, unix.TCIFLUSH) // FREAD, which TCIFLUSH equals here
}

// guardExecutable is this program's binary, which runs again as the guard
func guardExecutable() (string, error) { return os.Executable() }

// nameGuard leaves the guard's process name as it is: FreeBSD names a process
// after its binary, and the guard's arguments name it
func nameGuard() {}

// consolePath is the console's device, for the guard to open it again
func consolePath(tty *os.File) string {
	vt, err := unix.IoctlGetInt(int(tty.Fd()), vtGetIndex)
	if err != nil || vt < 1 {
		return ""
	}
	return "/dev/ttyv" + strconv.FormatInt(int64(vt-1), 32)
}

// guardRestore switches the console keyboard back for the guard, then has
// rest restore the console's tty. A session whose leader ends revokes its
// console, the game's leaving with it, which then takes no ioctl (ENOTTY):
// the guard opens it again, which vt needs no more of to switch the keyboard.
func guardRestore(tty *os.File, path string, mode int, rest func(fd int)) {
	fd := int(tty.Fd())
	switch err := unix.IoctlSetInt(fd, kdSetKeyboardMode, mode); err {
	case unix.ENOTTY, unix.ENXIO, unix.EBADF:
		guardReopen(path, mode, func(fd int) {
			flushInput(fd)
			rest(fd)
		})
	case nil:
		flushInput(fd)
		fallthrough
	default:
		rest(fd)
	}
}
