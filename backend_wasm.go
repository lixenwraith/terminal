//go:build wasm

package terminal

import (
	"os"
	"syscall/js"
	"time"
)

type wasmBackend struct {
	width, height int
	inputCh       chan []byte
	wake          chan struct{}
	jsCallbacks   []js.Func
}

func newBackend() Backend {
	return &wasmBackend{
		width:   80,
		height:  24,
		inputCh: make(chan []byte, 256),
		wake:    make(chan struct{}, 1),
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

// Read takes what the page sent; a lone ESC waits out the reader's wait, which
// ends it as Escape
func (b *wasmBackend) Read(wait time.Duration) ([]byte, error) {
	var passed <-chan time.Time
	if wait >= 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		passed = t.C
	}
	select {
	case data := <-b.inputCh:
		return data, nil
	case <-b.wake:
	case <-passed:
	}
	return nil, nil
}

func (b *wasmBackend) Wake() {
	select {
	case b.wake <- struct{}{}:
	default:
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
