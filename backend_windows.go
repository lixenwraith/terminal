//go:build windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pollWait is how often a read checks for Wake while it waits on the console
const pollWait = 10 * time.Millisecond

type windowsBackend struct {
	stdin  windows.Handle
	stdout windows.Handle
	opened []windows.Handle // CONIN$ or CONOUT$, opened for a redirected end

	oldStdinMode  uint32
	oldStdoutMode uint32
	oldInputCP    uint32
	oldOutputCP   uint32

	// The resize handler and the size it was last given, which the poll and the
	// console's size records both report through
	sizeMu sync.Mutex
	resize func(int, int)
	w, h   int

	woken atomic.Bool // Wake was called since a read last returned for it

	resizeStopCh chan struct{}
	resizeDoneCh chan struct{}
}

func newBackend() Backend {
	return &windowsBackend{
		stdin:  windows.Handle(os.Stdin.Fd()),
		stdout: windows.Handle(os.Stdout.Fd()),
	}
}

func (b *windowsBackend) Init() (err error) {
	if err := b.openConsole(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			b.closeConsole()
		}
	}()
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
	// any-event tracking on its own. Size records report a resize as it happens.
	stdinMode := uint32(windows.ENABLE_EXTENDED_FLAGS |
		windows.ENABLE_WINDOW_INPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	if err := windows.SetConsoleMode(b.stdin, stdinMode); err != nil {
		return fmt.Errorf("SetConsoleMode stdin, VT input (a legacy console?): %w", err)
	}

	stdoutMode := uint32(windows.ENABLE_PROCESSED_OUTPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING |
		windows.DISABLE_NEWLINE_AUTO_RETURN)
	if err := windows.SetConsoleMode(b.stdout, stdoutMode); err != nil {
		windows.SetConsoleMode(b.stdin, b.oldStdinMode)
		return fmt.Errorf("SetConsoleMode stdout, VT processing (a legacy console?): %w", err)
	}

	windows.SetConsoleCP(65001)
	windows.SetConsoleOutputCP(65001)

	return nil
}

// openConsole draws on the console itself in place of a redirected stdin or
// stdout, as /dev/tty does on Unix, while a std handle left on the console shows
// it is the one the player sees: with all three redirected (mintty without
// ConPTY) the process's console is a hidden one
func (b *windowsBackend) openConsole() error {
	var mode uint32
	for _, end := range []struct {
		h    *windows.Handle
		name string
	}{{&b.stdin, "CONIN$"}, {&b.stdout, "CONOUT$"}} {
		if windows.GetConsoleMode(*end.h, &mode) == nil {
			continue
		}
		if !consoleShown() {
			b.closeConsole()
			return errors.New("no console: stdin, stdout and stderr are redirected")
		}
		h, err := openConsoleDev(end.name)
		if err != nil {
			b.closeConsole()
			return fmt.Errorf("open %s: %w", end.name, err)
		}
		*end.h = h
		b.opened = append(b.opened, h)
	}
	return nil
}

// closeConsole closes what openConsole opened and returns to the std handles
func (b *windowsBackend) closeConsole() {
	for _, h := range b.opened {
		windows.CloseHandle(h)
	}
	b.opened = nil
	b.stdin, b.stdout = windows.Handle(os.Stdin.Fd()), windows.Handle(os.Stdout.Fd())
}

func (b *windowsBackend) Fini() {
	if b.resizeStopCh != nil {
		close(b.resizeStopCh)
		<-b.resizeDoneCh
		b.resizeStopCh = nil
	}
	b.sizeMu.Lock()
	b.resize = nil
	b.sizeMu.Unlock()
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
	b.closeConsole()
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

// Read waits on the console a poll at a time, for Wake and wait to end it, and
// returns only with input, a Wake, or wait passed
func (b *windowsBackend) Read(wait time.Duration) ([]byte, error) {
	buf := make([]byte, 256)
	deadline := time.Now().Add(wait)
	for {
		if b.woken.Swap(false) {
			return nil, nil
		}
		step := pollWait
		if wait >= 0 {
			left := time.Until(deadline)
			if left <= 0 {
				return nil, nil
			}
			step = min(step, left+time.Millisecond-1)
		}

		ev, err := windows.WaitForSingleObject(b.stdin, uint32(step/time.Millisecond))
		if err != nil {
			return nil, fmt.Errorf("WaitForSingleObject: %w", err)
		}
		if ev == uint32(windows.WAIT_TIMEOUT) {
			continue
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
			continue
		}
		return buf[:n], nil
	}
}

func (b *windowsBackend) Wake() { b.woken.Store(true) }

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
	skip, sized := uint32(0), false
	for skip < n && (recs[skip].kind != windows.KEY_EVENT || recs[skip].down == 0) {
		sized = sized || recs[skip].kind == windows.WINDOW_BUFFER_SIZE_EVENT
		skip++
	}
	if skip > 0 {
		if r, _, err := procReadConsoleInput.Call(uintptr(b.stdin), uintptr(unsafe.Pointer(&recs[0])), uintptr(skip), uintptr(unsafe.Pointer(&n))); r == 0 {
			return false, err
		}
		if sized {
			b.noteSize()
		}
		return false, nil
	}
	return n > 0, nil
}

// noteSize gives the resize handler the window's size if it changed since the
// handler was last given one. The size record says a resize happened; the size is
// the window's, which the record does not carry where the buffer outgrows it.
func (b *windowsBackend) noteSize() {
	b.sizeMu.Lock()
	defer b.sizeMu.Unlock()
	if b.resize == nil {
		return
	}
	if w, h := b.Size(); w != b.w || h != b.h {
		b.w, b.h = w, h
		b.resize(w, h)
	}
}

// SetResizeHandler reports a resize as the console's size record arrives; the poll
// catches a record ReadFile took on its way to a key, which it discards
func (b *windowsBackend) SetResizeHandler(handler func(int, int)) {
	b.sizeMu.Lock()
	b.resize = handler
	b.w, b.h = b.Size()
	b.sizeMu.Unlock()
	b.resizeStopCh = make(chan struct{})
	b.resizeDoneCh = make(chan struct{})

	go func() {
		defer close(b.resizeDoneCh)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-b.resizeStopCh:
				return
			case <-ticker.C:
				b.noteSize()
			}
		}
	}()
}
