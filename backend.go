package terminal

// Backend abstracts platform-specific terminal operations.
// This interface allows the terminal package to support both
// native Unix environments and WASM/Browser environments (via xterm.js).
type Backend interface {
	// Lifecycle
	Init() error
	Fini()

	// Capabilities
	Size() (width, height int)

	// I/O
	// Write writes raw bytes to the terminal output.
	Write(p []byte) error

	// Read blocks until input is available, the stop channel is closed, or an error occurs.
	Read(stopCh <-chan struct{}) ([]byte, error)

	// Callbacks
	// SetResizeHandler registers a callback for terminal resize events.
	SetResizeHandler(handler func(width, height int))
}