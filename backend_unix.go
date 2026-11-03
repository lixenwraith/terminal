//go:build unix

package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type unixBackend struct {
	in      *os.File
	out     *os.File
	inFd    int
	outFd   int
	tty     *os.File // the controlling terminal, opened for a redirected end
	oldTerm *term.State
	console keyConsole // the console input comes from, nil off one
	buf     [256]byte  // what Read returns, valid until the next Read

	// The pipe Wake writes to and Read polls beside the input, -1 while closed
	wakeMu sync.Mutex
	wake   [2]int

	resizeStopCh chan struct{}
	resizeDoneCh chan struct{}
}

func newBackend() Backend {
	return &unixBackend{
		in:    os.Stdin,
		out:   os.Stdout,
		inFd:  int(os.Stdin.Fd()),
		outFd: int(os.Stdout.Fd()),
		wake:  [2]int{-1, -1},
	}
}

// Init draws on the controlling terminal in place of a redirected stdin or
// stdout, so `app > file` and `cmd | app` keep both as data
func (b *unixBackend) Init() error {
	if os.Getenv("TERM") == "dumb" {
		return errors.New("TERM=dumb cannot address the cursor")
	}
	if !term.IsTerminal(b.inFd) || !term.IsTerminal(b.outFd) {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("no terminal: %w", err)
		}
		b.tty = tty
		if !term.IsTerminal(b.inFd) {
			b.in, b.inFd = tty, int(tty.Fd())
		}
		if !term.IsTerminal(b.outFd) {
			b.out, b.outFd = tty, int(tty.Fd())
		}
	}

	if err := b.openWake(); err != nil {
		b.closeTTY()
		return err
	}
	cooked, _ := unix.IoctlGetTermios(b.inFd, tcgets)
	old, err := term.MakeRaw(b.inFd)
	if err != nil {
		b.closeWake()
		b.closeTTY()
		return err
	}
	b.oldTerm = old
	b.console = openConsole(b.in, cooked)
	return nil
}

// closeTTY closes the controlling terminal Init opened, if any
func (b *unixBackend) closeTTY() {
	if b.tty != nil {
		b.tty.Close()
		b.tty = nil
	}
}

func (b *unixBackend) Fini() {
	if b.resizeStopCh != nil {
		close(b.resizeStopCh)
		<-b.resizeDoneCh
		b.resizeStopCh = nil
	}
	if b.console != nil {
		b.console.close()
		b.console = nil
	}
	if b.oldTerm != nil {
		term.Restore(b.inFd, b.oldTerm)
	}
	b.closeWake()
	b.closeTTY()
}

// openWake makes the wake pipe, both ends non-blocking and closed on exec
func (b *unixBackend) openWake() error {
	var p [2]int
	syscall.ForkLock.RLock()
	err := unix.Pipe(p[:])
	if err == nil {
		unix.CloseOnExec(p[0])
		unix.CloseOnExec(p[1])
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return fmt.Errorf("wake pipe: %w", err)
	}
	for _, fd := range p {
		if err = unix.SetNonblock(fd, true); err != nil {
			unix.Close(p[0])
			unix.Close(p[1])
			return fmt.Errorf("wake pipe: %w", err)
		}
	}
	b.wakeMu.Lock()
	b.wake = p
	b.wakeMu.Unlock()
	return nil
}

func (b *unixBackend) closeWake() {
	b.wakeMu.Lock()
	defer b.wakeMu.Unlock()
	if b.wake[0] >= 0 {
		unix.Close(b.wake[0])
		unix.Close(b.wake[1])
		b.wake = [2]int{-1, -1}
	}
}

// Wake writes to the wake pipe; one already full holds a wake that is pending
func (b *unixBackend) Wake() {
	b.wakeMu.Lock()
	defer b.wakeMu.Unlock()
	if b.wake[1] >= 0 {
		unix.Write(b.wake[1], []byte{0})
	}
}

// Size delegates to exported WindowSize; getTerminalSize deleted
func (b *unixBackend) Size() (int, int) {
	if w, h, ok := WindowSize(b.out); ok {
		return w, h
	}
	return 80, 24 // Fallback
}

// WindowSize queries terminal dimensions for f without raw mode or
// Terminal lifecycle. ok=false when f is not a terminal.
func WindowSize(f *os.File) (w, h int, ok bool) {
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 0, 0, false
	}
	return int(ws.Col), int(ws.Row), true
}

func (b *unixBackend) Write(p []byte) error {
	_, err := b.out.Write(p)
	return err
}

// Read polls the input beside the wake pipe, so a read with no limit waits on
// the terminal without a wakeup until input or Wake
func (b *unixBackend) Read(wait time.Duration) ([]byte, error) {
	b.wakeMu.Lock()
	fds := []unix.PollFd{{Fd: int32(b.inFd), Events: unix.POLLIN}, {Fd: int32(b.wake[0]), Events: unix.POLLIN}}
	b.wakeMu.Unlock()
	deadline := time.Now().Add(wait)
	for {
		ms := -1
		if wait >= 0 {
			ms = int(max(time.Until(deadline)+time.Millisecond-1, 0) / time.Millisecond)
		}
		n, err := unix.Poll(fds, ms)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		if fds[1].Revents != 0 {
			var drain [64]byte
			for n, _ := unix.Read(int(fds[1].Fd), drain[:]); n == len(drain); n, _ = unix.Read(int(fds[1].Fd), drain[:]) {
			}
			return nil, nil
		}

		rn, err := unix.Read(b.inFd, b.buf[:])
		if err != nil {
			if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
				continue
			}
			return nil, err
		}
		if rn == 0 {
			return nil, io.EOF // the terminal hung up
		}
		return b.buf[:rn], nil
	}
}

// keySources: the console's keyboard on a console, and the key protocol,
// which the terminal is asked about
func (b *unixBackend) keySources() KeySource {
	if b.console != nil {
		return KeySourceConsole | KeySourceProtocol
	}
	return KeySourceProtocol
}

// keyConsole is the console input comes from, nil off one
func (b *unixBackend) keyConsole() keyConsole { return b.console }

func (b *unixBackend) SetResizeHandler(handler func(width, height int)) {
	b.resizeStopCh = make(chan struct{})
	b.resizeDoneCh = make(chan struct{})

	go func() {
		defer close(b.resizeDoneCh)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		defer signal.Stop(sigCh)

		for {
			select {
			case <-b.resizeStopCh:
				return
			case <-sigCh:
				w, h := b.Size()
				handler(w, h)
			}
		}
	}()
}
