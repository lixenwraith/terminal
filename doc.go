// Package terminal provides direct ANSI terminal control with zero-alloc rendering.
//
// # Features
//
//   - True color (24-bit) and 256-color palette support
//   - Double-buffered output with cell-level diffing
//   - Raw stdin input parsing with escape sequence handling
//   - Resize detection (SIGWINCH on Unix, callback on WASM)
//   - Clean terminal restoration on exit/panic
//
// # Platform Support
//
// The package uses build tags to separate platform-specific code:
//
//   - unix: Native terminal via termios, unix.Poll, SIGWINCH
//   - wasm: Browser terminal via xterm.js JavaScript bridge
//
// # Architecture
//
// The Backend interface abstracts platform-specific operations:
//
//	Backend (interface)
//	├── unixBackend  (//go:build unix)  - termios, raw I/O, signals
//	└── wasmBackend  (//go:build wasm)  - syscall/js, callbacks
//
// Shared code (no build tags): Terminal interface, cell diffing, ANSI generation,
// escape sequence parsing, service lifecycle.
//
// # WASM Integration
//
// WASM builds require JavaScript glue exposing these globals:
//
//	goTerminalWrite(Uint8Array)     // Go → JS: terminal output
//	goTerminalInput(Uint8Array)     // JS → Go: keyboard input
//	goTerminalResize(cols, rows)    // JS → Go: terminal resize
//	xterm.cols, xterm.rows          // Initial size query
//
// # Performance
//
// Output uses 128KB buffered writer with cell-level diffing. Only changed cells
// generate ANSI sequences. Style attributes are coalesced to minimize SGR calls.
// Input parsing is zero-allocation for common cases.
//
// This package bypasses terminfo/termcap entirely, emitting direct ANSI sequences.
// Target environments: Linux, macOS, BSDs with xterm-compatible terminals, and
// modern browsers with xterm.js.
package terminal

