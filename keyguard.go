//go:build linux

package terminal

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The guard restores a console keyboard the game leaves raw by dying where no
// handler of its own runs: SIGKILL, the OOM killer, a Go fatal error. Before
// the first raw switch the package runs its program's binary again with
// guardEnv set, which the package's init catches before main. The guard keeps
// the console and the read end of a pipe whose write end only the game holds:
// the pipe's end is the game's, and if the keyboard is raw then, the guard
// switches it back, restores the termios and shows the cursor.

// guardEnv marks a run of the binary as a guard, which ps lists as guardName
const (
	guardEnv  = "LIXENWRAITH_TERMINAL_GUARD"
	guardName = "console-guard"
)

// The guard's messages: armed with the keyboard mode and termios to restore,
// or disarmed
const (
	guardArm    = 'A'
	guardDisarm = 'D'
)

func init() {
	if os.Getenv(guardEnv) == "1" {
		nameGuard()
		os.Exit(runGuard(os.NewFile(3, "guard"), os.NewFile(4, "console")))
	}
}

// keyGuard is the game's end of its guard
type keyGuard struct {
	w *os.File // the pipe's write end, closed on exec in any other child
}

// startGuard runs the guard over tty
func startGuard(tty *os.File) (*keyGuard, error) {
	exe, err := guardExecutable()
	if err != nil {
		return nil, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	// Not named after the game, so killing the game by name spares it
	cmd := exec.Command(exe)
	cmd.Args = []string{guardName}
	cmd.Env = append(os.Environ(), guardEnv+"=1")
	cmd.ExtraFiles = []*os.File{r, tty}
	// A group of its own, so a kill of the game's job leaves it to restore.
	// A game leading its session takes the console with it as it dies, the
	// kernel hanging the console up: its guard leads a session of its own, to
	// take the console up again.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if sid, err := unix.Getsid(0); err == nil && sid == os.Getpid() {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		w.Close()
		return nil, err
	}
	go cmd.Wait()
	return &keyGuard{w: w}, nil
}

// termiosBytes is t as the guard reads it, the same binary on both ends
func termiosBytes(t *unix.Termios) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(t)), unsafe.Sizeof(*t))
}

// arm tells the guard the keyboard is about to go raw, and what restores it;
// false when the guard is gone. One write, under the pipe's atomic size.
func (g *keyGuard) arm(mode int, cooked *unix.Termios) bool {
	_, err := g.w.Write(append([]byte{guardArm, byte(mode)}, termiosBytes(cooked)...))
	return err == nil
}

// disarm tells the guard the keyboard is cooked again
func (g *keyGuard) disarm() { g.w.Write([]byte{guardDisarm}) }

// close ends the guard, which exits disarmed
func (g *keyGuard) close() { g.w.Close() }

// guardReset is what the guard writes after restoring the keyboard: mouse
// reports off, the cursor shown, attributes reset, but no RIS, which would
// clear the message the game died with
var guardReset = string(csiMouseMotionOff) + string(csiMouseDragOff) + string(csiMouseClickOff) +
	string(csiMouseSGROff) + string(csiPasteOff) + string(csiCursorShow) + string(csiSGR0) + string(csiAutoWrapOn)

// runGuard waits on cmds until the game is gone, and restores the console
// tty if the keyboard was raw then
func runGuard(cmds, tty *os.File) int {
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM,
		syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU, syscall.SIGPIPE)
	armed, mode, cooked := guardWait(cmds)
	if armed {
		guardRestore(tty, mode, func(fd int) {
			unix.IoctlSetTermios(fd, tcsets, &cooked)
			unix.Write(fd, []byte(guardReset))
		})
	}
	return 0
}

// guardWait reads the game's messages to their end: whether the guard was
// armed then, and with what
func guardWait(cmds io.Reader) (armed bool, mode int, cooked unix.Termios) {
	var msg [1]byte
	arg := make([]byte, 1+len(termiosBytes(&cooked)))
	for {
		if _, err := io.ReadFull(cmds, msg[:]); err != nil {
			return armed, mode, cooked
		}
		switch msg[0] {
		case guardArm:
			if _, err := io.ReadFull(cmds, arg); err != nil {
				return armed, mode, cooked
			}
			armed, mode = true, int(arg[0])
			copy(termiosBytes(&cooked), arg[1:])
		case guardDisarm:
			armed = false
		}
	}
}
