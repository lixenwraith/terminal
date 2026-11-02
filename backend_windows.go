//go:build windows

package terminal

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const escapeTimeoutMs = 10

type windowsBackend struct {
	stdin  windows.Handle
	stdout windows.Handle

	oldStdinMode  uint32
	oldStdoutMode uint32
	oldInputCP    uint32
	oldOutputCP   uint32

	resizeStopCh chan struct{}
	resizeDoneCh chan struct{}
}

func newBackend() Backend {
	return &windowsBackend{
		stdin:  windows.Handle(os.Stdin.Fd()),
		stdout: windows.Handle(os.Stdout.Fd()),
	}
}

func (b *windowsBackend) Init() error {
	if err := windows.GetConsoleMode(b.stdin, &b.oldStdinMode); err != nil {
		return fmt.Errorf("GetConsoleMode stdin: %w", err)
	}
	if err := windows.GetConsoleMode(b.stdout, &b.oldStdoutMode); err != nil {
		return fmt.Errorf("GetConsoleMode stdout: %w", err)
	}
	b.oldInputCP, _ = windows.GetConsoleCP()
	b.oldOutputCP, _ = windows.GetConsoleOutputCP()

	// Mouse reports arrive as VT text once asked for; ENABLE_MOUSE_INPUT would add
	// MOUSE records ReadFile discards, and has a newer ConPTY ask the terminal for
	// any-event tracking on its own
	stdinMode := uint32(windows.ENABLE_EXTENDED_FLAGS |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	if err := windows.SetConsoleMode(b.stdin, stdinMode); err != nil {
		return fmt.Errorf("SetConsoleMode stdin: %w", err)
	}

	stdoutMode := uint32(windows.ENABLE_PROCESSED_OUTPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING |
		windows.DISABLE_NEWLINE_AUTO_RETURN)
	if err := windows.SetConsoleMode(b.stdout, stdoutMode); err != nil {
		windows.SetConsoleMode(b.stdin, b.oldStdinMode)
		return fmt.Errorf("SetConsoleMode stdout: %w", err)
	}

	windows.SetConsoleCP(65001)
	windows.SetConsoleOutputCP(65001)

	return nil
}

func (b *windowsBackend) Fini() {
	if b.resizeStopCh != nil {
		close(b.resizeStopCh)
		<-b.resizeDoneCh
		b.resizeStopCh = nil
	}
	if b.oldStdinMode != 0 {
		windows.SetConsoleMode(b.stdin, b.oldStdinMode)
	}
	if b.oldStdoutMode != 0 {
		windows.SetConsoleMode(b.stdout, b.oldStdoutMode)
	}
	if b.oldInputCP != 0 {
		windows.SetConsoleCP(b.oldInputCP)
	}
	if b.oldOutputCP != 0 {
		windows.SetConsoleOutputCP(b.oldOutputCP)
	}
}

func windowSizeHandle(h windows.Handle) (int, int, bool) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return 0, 0, false
	}
	w := int(info.Window.Right-info.Window.Left) + 1
	ht := int(info.Window.Bottom-info.Window.Top) + 1
	if w < 1 || ht < 1 {
		return 0, 0, false
	}
	return w, ht, true
}

// WindowSize queries terminal dimensions for f without console mode changes.
// ok=false when f is not a console handle.
func WindowSize(f *os.File) (w, h int, ok bool) {
	return windowSizeHandle(windows.Handle(f.Fd()))
}

func (b *windowsBackend) Size() (int, int) {
	if w, h, ok := windowSizeHandle(b.stdout); ok {
		return w, h
	}
	return 80, 24
}

func (b *windowsBackend) Write(p []byte) error {
	var written uint32
	return windows.WriteFile(b.stdout, p, &written, nil)
}

func (b *windowsBackend) Read(stopCh <-chan struct{}) ([]byte, error) {
	buf := make([]byte, 256)
	for {
		select {
		case <-stopCh:
			return nil, nil
		default:
		}

		ev, err := windows.WaitForSingleObject(b.stdin, uint32(escapeTimeoutMs))
		if err != nil {
			return nil, fmt.Errorf("WaitForSingleObject: %w", err)
		}
		if ev == uint32(windows.WAIT_TIMEOUT) {
			// Mirrors unix poll timeout: lets readLoop emit pending standalone ESC
			return nil, nil
		}
		if ok, err := b.keyWaiting(); err != nil {
			return nil, fmt.Errorf("PeekConsoleInputW: %w", err)
		} else if !ok {
			continue
		}

		var n uint32
		if err := windows.ReadFile(b.stdin, buf, &n, nil); err != nil {
			return nil, fmt.Errorf("ReadFile: %w", err)
		}
		if n == 0 {
			// VTP consumed a non-keyboard record (e.g. focus event) producing no bytes
			return nil, nil
		}

		ret := make([]byte, n)
		copy(ret, buf[:n])
		return ret, nil
	}
}

var (
	procPeekConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("PeekConsoleInputW")
	procReadConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")
)

// inputRecord is INPUT_RECORD: the event type, then its 16-byte union, which for
// a KEY_EVENT starts with bKeyDown
type inputRecord struct {
	kind uint16
	_    uint16
	down uint32
	_    [12]byte
}

// keyWaiting takes the records queued ahead of the first key press and reports
// whether one waits: ReadFile passes over a focus, menu or mouse record or a
// key's release only to wait for a character, which would hold the reader past
// the timeout that sends a lone ESC, and past Fini
func (b *windowsBackend) keyWaiting() (bool, error) {
	var recs [16]inputRecord
	var n uint32
	if r, _, err := procPeekConsoleInput.Call(uintptr(b.stdin), uintptr(unsafe.Pointer(&recs[0])), uintptr(len(recs)), uintptr(unsafe.Pointer(&n))); r == 0 {
		return false, err
	}
	skip := uint32(0)
	for skip < n && (recs[skip].kind != windows.KEY_EVENT || recs[skip].down == 0) {
		skip++
	}
	if skip > 0 {
		if r, _, err := procReadConsoleInput.Call(uintptr(b.stdin), uintptr(unsafe.Pointer(&recs[0])), uintptr(skip), uintptr(unsafe.Pointer(&n))); r == 0 {
			return false, err
		}
		return false, nil
	}
	return n > 0, nil
}

func (b *windowsBackend) SetResizeHandler(handler func(int, int)) {
	b.resizeStopCh = make(chan struct{})
	b.resizeDoneCh = make(chan struct{})

	go func() {
		defer close(b.resizeDoneCh)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		w, h := b.Size()
		for {
			select {
			case <-b.resizeStopCh:
				return
			case <-ticker.C:
				nw, nh := b.Size()
				if nw != w || nh != h {
					w, h = nw, nh
					handler(w, h)
				}
			}
		}
	}()
}
