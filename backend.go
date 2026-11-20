package terminal

import "time"

// Backend is the terminal a Terminal writes bytes to and reads them from: the
// tty or console New opens, or a caller's own for NewWithBackend
type Backend interface {
	Init() error
	Fini()

	Size() (width, height int)

	Write(p []byte) error

	// Read blocks until input is available, wait passes or Wake is called, and
	// returns no data for the last two; a negative wait never passes. The data
	// is valid until the next Read.
	Read(wait time.Duration) ([]byte, error)

	// Wake makes the Read in progress return, or the next one if none is.
	Wake()

	// SetResizeHandler is called once, after Init, with what a new size goes to
	SetResizeHandler(handler func(width, height int))
}
