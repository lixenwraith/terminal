package terminal

import "time"

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

	// Read blocks until input is available, wait passes or Wake is called, and
	// returns no data for the last two; a negative wait never passes. The data
	// is valid until the next Read.
	Read(wait time.Duration) ([]byte, error)

	// Wake makes the Read in progress return, or the next one if none is.
	Wake()

	// Callbacks
	// SetResizeHandler registers a callback for terminal resize events.
	SetResizeHandler(handler func(width, height int))
}
