//go:build wasm

package terminal

import (
	"os"
	"syscall/js"
)

type wasmBackend struct {
	width, height   int
	inputCh         chan []byte
	jsCallbacks     []js.Func
	returnEmptyNext bool // Signal to return empty on next Read() for standalone ESC
}

const escapeTimeoutMs = 10

func newBackend() Backend {
	return &wasmBackend{
		width:   80,
		height:  24,
		inputCh: make(chan []byte, 256),
	}
}

func (b *wasmBackend) Init() error {
	// Register JS callbacks
	inputCb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			data := make([]byte, args[0].Length())
			js.CopyBytesToGo(data, args[0])
			select {
			case b.inputCh <- data:
			default:
				// Buffer full, drop input
			}
		}
		return nil
	})
	b.jsCallbacks = append(b.jsCallbacks, inputCb)
	js.Global().Set("goTerminalInput", inputCb)

	resizeCb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			w, h := args[0].Int(), args[1].Int()
			b.width, b.height = w, h
			// Resize handler is set via SetResizeHandler, but we need to store it
			// or have this callback call a method. For simplicity, we'll assign
			// the handler to a struct field if we need dynamic updates,
			// but here we rely on the struct field set by SetResizeHandler.
			// However, SetResizeHandler might be called after Init.
			// See below for corrected flow.
		}
		return nil
	})
	b.jsCallbacks = append(b.jsCallbacks, resizeCb)
	js.Global().Set("goTerminalResize", resizeCb)

	// Initial size query
	if xterm := js.Global().Get("xterm"); !xterm.IsUndefined() {
		b.width = xterm.Get("cols").Int()
		b.height = xterm.Get("rows").Int()
	}

	return nil
}

func (b *wasmBackend) Fini() {
	for _, cb := range b.jsCallbacks {
		cb.Release()
	}
	js.Global().Delete("goTerminalInput")
	js.Global().Delete("goTerminalResize")
}

func (b *wasmBackend) Size() (int, int) {
	return b.width, b.height
}

// WindowSize queries xterm.js dimensions. The file argument is ignored;
// WASM has a single terminal. ok=false when the xterm global is absent.
func WindowSize(_ *os.File) (w, h int, ok bool) {
	xterm := js.Global().Get("xterm")
	if xterm.IsUndefined() || xterm.IsNull() {
		return 0, 0, false
	}
	return xterm.Get("cols").Int(), xterm.Get("rows").Int(), true
}

func (b *wasmBackend) Write(p []byte) error {
	arr := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(arr, p)
	js.Global().Call("goTerminalWrite", arr)
	return nil
}

func (b *wasmBackend) Read(stopCh <-chan struct{}) ([]byte, error) {
	// After standalone ESC timeout, return empty to trigger readLoop's ESC emission
	if b.returnEmptyNext {
		b.returnEmptyNext = false
		return nil, nil
	}

	select {
	case data := <-b.inputCh:
		// If we received exactly ESC, wait briefly for more data
		// (in case it's start of escape sequence split across callbacks)
		if len(data) == 1 && data[0] == 0x1b {
			// Use JS setTimeout via a promise-based wait
			moreCh := make(chan []byte, 1)

			// Schedule timeout callback
			var timeoutCb js.Func
			timeoutCb = js.FuncOf(func(_ js.Value, _ []js.Value) any {
				select {
				case moreCh <- nil:
				default:
				}
				timeoutCb.Release()
				return nil
			})
			js.Global().Call("setTimeout", timeoutCb, escapeTimeoutMs)

			// Wait for more data or timeout
			select {
			case more := <-b.inputCh:
				// More data arrived, combine
				return append(data, more...), nil
			case <-moreCh:
				// Timeout, standalone ESC confirmed
				// Signal next Read() to return empty (triggers readLoop standalone ESC logic)
				b.returnEmptyNext = true
				return data, nil
			case <-stopCh:
				return nil, nil
			}
		}
		return data, nil
	case <-stopCh:
		return nil, nil
	}
}

func (b *wasmBackend) SetResizeHandler(handler func(width, height int)) {
	// Overwrite the resize callback to include the handler invocation
	// This ensures the handler acts on the latest registration
	resizeCb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			w, h := args[0].Int(), args[1].Int()
			b.width, b.height = w, h
			handler(w, h)
		}
		return nil
	})
	b.jsCallbacks = append(b.jsCallbacks, resizeCb)
	js.Global().Set("goTerminalResize", resizeCb)
}

