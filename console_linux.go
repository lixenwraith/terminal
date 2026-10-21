//go:build linux

package terminal

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The Linux console's ioctls (linux/kd.h, linux/vt.h, linux/tiocl.h), which
// golang.org/x/sys/unix does not define
const (
	kdGetKeyboardMode  = 0x4B44 // KDGKBMODE
	kdSetKeyboardMode  = 0x4B45 // KDSKBMODE
	kdGetKeymapEntry   = 0x4B46 // KDGKBENT
	kdGetFunctionKey   = 0x4B48 // KDGKBSENT
	kdGetAccents       = 0x4BFA // KDGKBDIACRUC
	kdGetMetaMode      = 0x4B62 // KDGKBMETA
	kdGetLockFlags     = 0x4B64 // KDGKBLED
	kdSetLockFlags     = 0x4B65 // KDSKBLED
	vtGetState         = 0x5603 // VT_GETSTATE
	vtActivate         = 0x5606 // VT_ACTIVATE
	tioclGetShiftState = 6      // TIOCLINUX subcode: the modifiers down
	ttyMajor           = 4      // the consoles' device major, minors 1 to 63
)

// Keyboard modes, and the meta mode that types Meta keys as ESC prefixes
const (
	kXlate     = 0x01 // K_XLATE
	kMediumRaw = 0x02 // K_MEDIUMRAW
	kUnicode   = 0x03 // K_UNICODE
	kEscPrefix = 0x04 // K_ESCPREFIX
)

type kbEntry struct {
	table, index uint8
	value        uint16
}

type kbSEntry struct {
	fn  uint8
	str [512]byte
}

type kbDiacrsUC struct {
	count   uint32
	entries [256]struct{ diacr, base, result uint32 }
}

type vtStat struct {
	active, signal, state uint16
}

// linuxConsole is the Linux console the game runs on, its keyboard switched
// to K_MEDIUMRAW while releases come from it
type linuxConsole struct {
	tty     *os.File
	fd      int
	vt      int           // the console's number, from 1
	termios *unix.Termios // the termios the backend found, which the guard restores

	mu      sync.Mutex
	mode    int // the keyboard mode prepare found, which cooked restores
	isRaw   bool
	km      linuxKeymap
	kb      linuxKeys
	defLock uint8 // the default lock flags, beside the flags in KDGKBLED's answer
	dec     mediumRaw
	away    bool // another console had the keyboard when last asked
	guard   *keyGuard
}

// openConsole is the console tty is, nil unless it is one: a Linux virtual
// console, not a pty, a serial line or /dev/console
func openConsole(tty *os.File, cooked *unix.Termios) keyConsole {
	fd := int(tty.Fd())
	var st unix.Stat_t
	if cooked == nil || unix.Fstat(fd, &st) != nil || unix.Major(st.Rdev) != ttyMajor {
		return nil
	}
	vt := int(unix.Minor(st.Rdev))
	if vt < 1 || vt > 63 {
		return nil
	}
	if _, err := unix.IoctlGetInt(fd, kdGetKeyboardMode); err != nil {
		return nil
	}
	c := &linuxConsole{tty: tty, fd: fd, vt: vt, termios: cooked}
	c.kb.km = &c.km
	return c
}

func ioctlPtr(fd int, req uint, arg unsafe.Pointer) error {
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// prepare reads the keymap, starts the guard and lets output a Scroll Lock
// stopped go on; a keyboard some other program has raw is left alone
func (c *linuxConsole) prepare() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isRaw {
		return true
	}
	mode, err := unix.IoctlGetInt(c.fd, kdGetKeyboardMode)
	if err != nil || mode != kXlate && mode != kUnicode || !c.readKeymap() {
		return false
	}
	if c.guard == nil {
		if c.guard, err = startGuard(c.tty); err != nil {
			return false
		}
	}
	c.mode = mode
	unix.IoctlSetInt(c.fd, unix.TCXONC, unix.TCOON)
	return true
}

// readKeymap reads the kernel's keymap: each map's entries, the function key
// strings, the accent table and the meta mode. Entries are read cooked, while
// KDGKBENT still answers Unicode ones.
func (c *linuxConsole) readKeymap() bool {
	km := &c.km
	for table := range km.maps {
		e := kbEntry{table: uint8(table)}
		if ioctlPtr(c.fd, kdGetKeymapEntry, unsafe.Pointer(&e)) != nil {
			return false
		}
		if e.value == kNoSuchMap {
			km.maps[table] = nil
			continue
		}
		m := km.maps[table]
		if m == nil {
			m = new([nrKeys]uint16)
			km.maps[table] = m
		}
		m[0] = e.value ^ 0xf000
		for i := 1; i < nrKeys; i++ {
			e.index = uint8(i)
			if ioctlPtr(c.fd, kdGetKeymapEntry, unsafe.Pointer(&e)) != nil {
				return false
			}
			m[i] = e.value ^ 0xf000
		}
	}
	var s kbSEntry
	for fn := range km.funcs {
		s.fn, km.funcs[fn] = uint8(fn), ""
		if ioctlPtr(c.fd, kdGetFunctionKey, unsafe.Pointer(&s)) == nil {
			if n := strings.IndexByte(string(s.str[:]), 0); n >= 0 {
				km.funcs[fn] = string(s.str[:n])
			}
		}
	}
	var acc kbDiacrsUC
	km.accents = km.accents[:0]
	if ioctlPtr(c.fd, kdGetAccents, unsafe.Pointer(&acc)) == nil {
		for _, a := range acc.entries[:min(int(acc.count), len(acc.entries))] {
			km.accents = append(km.accents, linuxAccent{rune(a.diacr), rune(a.base), rune(a.result)})
		}
	}
	meta, err := unix.IoctlGetInt(c.fd, kdGetMetaMode)
	km.escMeta = err != nil || meta == kEscPrefix
	km.csiMods = linuxAtLeast(7, 1)
	return true
}

// linuxAtLeast is whether the running kernel is major.minor or later
func linuxAtLeast(major, minor int) bool {
	var u unix.Utsname
	if unix.Uname(&u) != nil {
		return false
	}
	release := unix.ByteSliceToString(u.Release[:])
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(strings.TrimRightFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' }))
	if err1 != nil || err2 != nil {
		return false
	}
	return maj > major || maj == major && min >= minor
}

// state reads the modifiers down and the lock flags translation starts from
func (c *linuxConsole) state() (shift, lock uint8) {
	arg := [1]byte{tioclGetShiftState}
	if ioctlPtr(c.fd, unix.TIOCLINUX, unsafe.Pointer(&arg[0])) == nil {
		shift = arg[0]
	}
	if flags, err := unix.IoctlGetInt(c.fd, kdGetLockFlags); err == nil {
		lock, c.defLock = uint8(flags&0x07), uint8(flags>>4&0x07)
	}
	return shift, lock
}

// raw arms the guard and switches the keyboard to K_MEDIUMRAW, which the
// kernel allows on the caller's controlling tty
func (c *linuxConsole) raw() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isRaw {
		return true
	}
	if c.guard == nil || !c.guard.arm(c.mode, c.termios) {
		return false
	}
	shift, lock := c.state()
	if unix.IoctlSetInt(c.fd, kdSetKeyboardMode, kMediumRaw) != nil {
		c.guard.disarm()
		return false
	}
	c.kb.seed(shift, lock)
	c.dec, c.away, c.isRaw = mediumRaw{}, false, true
	rawLinuxConsole.Store(&consoleRestore{fd: c.fd, mode: c.mode, guard: c.guard})
	return true
}

// cooked switches the keyboard back
func (c *linuxConsole) cooked() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookedLocked()
}

// cookedLocked switches the keyboard back; a console that refuses keeps the
// guard armed. Caller MUST hold mu.
func (c *linuxConsole) cookedLocked() {
	if !c.isRaw {
		return
	}
	c.isRaw = false
	rawLinuxConsole.Store(nil)
	if unix.IoctlSetInt(c.fd, kdSetKeyboardMode, c.mode) == nil {
		c.guard.disarm()
	}
}

// keys decodes raw keycodes and translates each key
func (c *linuxConsole) keys(data []byte, key func(code uint32, action KeyAction, typed []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isRaw {
		return
	}
	c.dec.decode(data, func(code int, down bool) {
		if code >= nrKeys {
			return // no keymap maps it
		}
		action := KeyRelease
		if down {
			action = KeyPress
			if c.kb.isDown(code) {
				action = KeyRepeat
			}
		}
		typed, act := c.kb.key(code, down, action == KeyRepeat)
		c.act(act)
		key(uint32(code), action, typed)
	})
}

// act does what a key asks of the console. Caller MUST hold mu.
func (c *linuxConsole) act(a linuxAct) {
	if a.leds {
		unix.IoctlSetInt(c.fd, kdSetLockFlags, int(c.kb.leds)|int(c.defLock)<<4)
	}
	if a.start {
		unix.IoctlSetInt(c.fd, unix.TCXONC, unix.TCOON)
	}
	if a.vt != 0 {
		c.switchTo(a.vt)
	}
}

// switchTo switches to console vt, or the one before or after this one in
// use, as the kernel does for a key cooked: only to a console in use, which
// VT_GETSTATE tells of the first 15
func (c *linuxConsole) switchTo(vt int) {
	var st vtStat
	if ioctlPtr(c.fd, vtGetState, unsafe.Pointer(&st)) != nil {
		return
	}
	cur, inUse := int(st.active), func(n int) bool { return n > 15 || st.state&(1<<n) != 0 }
	if vt < 0 && cur >= 1 && cur <= 15 {
		step := 1
		if vt == vtPrev {
			step = -1
		}
		vt = 0
		for n := (cur+step+14)%15 + 1; n != cur; n = (n+step+14)%15 + 1 {
			if inUse(n) {
				vt = n
				break
			}
		}
	}
	if vt > 0 && vt != cur && inUse(vt) {
		unix.IoctlSetInt(c.fd, vtActivate, vt)
	}
}

// front is whether this console is on screen; back on it, translation starts
// again from the console's state, its modifiers having moved elsewhere
func (c *linuxConsole) front() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	var st vtStat
	if !c.isRaw || ioctlPtr(c.fd, vtGetState, unsafe.Pointer(&st)) != nil {
		return true
	}
	front := int(st.active) == c.vt
	if front && c.away {
		c.kb.seed(c.state())
		c.dec = mediumRaw{}
	}
	c.away = !front
	return front
}

// close switches the keyboard back and ends the guard
func (c *linuxConsole) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookedLocked()
	if c.guard != nil {
		c.guard.close()
		c.guard = nil
	}
}

// consoleRestore is what restores a console keyboard left raw
type consoleRestore struct {
	fd, mode int
	guard    *keyGuard
}

// rawLinuxConsole is the console a Terminal has raw, for EmergencyReset
var rawLinuxConsole atomic.Pointer[consoleRestore]

// restoreConsoleKeys switches back a console keyboard a Terminal has raw. It
// takes no lock, for a reset from a panic that may hold one.
func restoreConsoleKeys() {
	if r := rawLinuxConsole.Swap(nil); r != nil && unix.IoctlSetInt(r.fd, kdSetKeyboardMode, r.mode) == nil {
		r.guard.disarm()
	}
}

// guardExecutable is this program's binary, which runs again as the guard:
// the running one, even replaced or deleted since
func guardExecutable() (string, error) { return "/proc/self/exe", nil }

// nameGuard names the guard's process as its arguments do; init runs on the
// main thread, whose name is the process's
func nameGuard() {
	name := append([]byte(guardName), 0)
	unix.Prctl(unix.PR_SET_NAME, uintptr(unsafe.Pointer(&name[0])), 0, 0, 0)
}

// guardReopenFor is how long the guard waits for a console its game took
// along to come free
const guardReopenFor = 2 * time.Second

// guardRestore switches the console keyboard back for the guard, then has
// rest restore the console's tty. A game that led the console's session took
// the console with it, the kernel hanging it up as the game died: the guard,
// leading a session of its own then, opens the console again once it is free,
// which makes it the guard's controlling tty and lets it switch the keyboard.
func guardRestore(tty *os.File, mode int, rest func(fd int)) {
	fd := int(tty.Fd())
	if err := unix.IoctlSetInt(fd, kdSetKeyboardMode, mode); err != unix.EIO && err != unix.EPERM {
		rest(fd)
		return
	}
	var st unix.Stat_t
	if sid, _ := unix.Getsid(0); sid != os.Getpid() || unix.Fstat(fd, &st) != nil {
		return
	}
	path := "/dev/tty" + strconv.Itoa(int(unix.Minor(st.Rdev)))
	for end := time.Now().Add(guardReopenFor); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if unix.IoctlSetInt(int(f.Fd()), kdSetKeyboardMode, mode) == nil {
			rest(int(f.Fd()))
			// Given up, the console isn't hung up again as the guard ends
			unix.IoctlSetInt(int(f.Fd()), unix.TIOCNOTTY, 0)
			f.Close()
			return
		}
		f.Close()
	}
}
