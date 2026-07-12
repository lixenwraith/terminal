//go:build unix

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type unixBackend struct {
	in      *os.File
	out     *os.File
	inFd    int
	outFd   int
	oldTerm *term.State

	resizeStopCh chan struct{}
	resizeDoneCh chan struct{}
}

const escapeTimeoutMs = 10

func newBackend() Backend {
	return &unixBackend{
		in:    os.Stdin,
		out:   os.Stdout,
		inFd:  int(os.Stdin.Fd()),
		outFd: int(os.Stdout.Fd()),
	}
}

func (b *unixBackend) Init() error {
	if !term.IsTerminal(b.inFd) {
		return fmt.Errorf("stdin is not a terminal")
	}

	old, err := term.MakeRaw(b.inFd)
	if err != nil {
		return err
	}
	b.oldTerm = old
	return nil
}

func (b *unixBackend) Fini() {
	if b.resizeStopCh != nil {
		close(b.resizeStopCh)
		<-b.resizeDoneCh
		b.resizeStopCh = nil
	}
	if b.oldTerm != nil {
		term.Restore(b.inFd, b.oldTerm)
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

// Read implements the polling logic previously in input.go
func (b *unixBackend) Read(stopCh <-chan struct{}) ([]byte, error) {
	// Buffer for single read
	buf := make([]byte, 256)

	for {
		select {
		case <-stopCh:
			return nil, nil
		default:
		}

		// Poll with timeout to allow checking stopCh
		fds := []unix.PollFd{
			{Fd: int32(b.inFd), Events: unix.POLLIN},
		}

		// Timeout to differentiate standalone ESC from escape sequences
		n, err := unix.Poll(fds, escapeTimeoutMs)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return nil, err
		}

		if n == 0 {
			// Timeout - return empty to let readLoop handle pending ESC
			return nil, nil
		}

		// Read data
		rn, err := unix.Read(b.inFd, buf)
		if err != nil {
			if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
				continue
			}
			return nil, err
		}

		if rn == 0 {
			// EOF
			return nil, nil
		}

		// Return copy of data
		ret := make([]byte, rn)
		copy(ret, buf[:rn])
		return ret, nil
	}
}

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
