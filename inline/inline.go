//go:build unix

// Package inline renders styled text and in-place progress in the normal
// terminal scrollback: no raw mode, no alternate screen, no input handling,
// no cursor hiding. Intended for CLI tools that want color and live status
// without owning the screen.
//
// Model: permanent lines scroll via Log; a live block of status lines is
// pinned below them and rewritten in place via Update. Done erases the
// block and optionally prints final permanent lines.
//
// Non-terminal output (pipes, CI): Update is a no-op, styling is stripped
// unless overridden with SetColor(true), Log and Done print plainly.
//
// Width is measured in runes (unicode/utf8); wide and combining characters
// are not width-aware — same documented limitation as tui.
package inline

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/lixenwraith/terminal"
)

// Printer manages styled output and the live block. Safe for concurrent use.
type Printer struct {
	mu    sync.Mutex
	w     *bufio.Writer
	tty   *os.File // non-nil when output is a terminal
	color bool
	mode  terminal.ColorMode
	live  []string // desired live block content
	drawn int      // lines currently on screen (may be clamped below len(live))
}

// New creates a Printer for w. Terminal detection via WindowSize probe;
// styling defaults on for terminals with NO_COLOR unset.
func New(w io.Writer) *Printer {
	p := &Printer{w: bufio.NewWriter(w)}
	if f, isFile := w.(*os.File); isFile {
		if _, _, ok := terminal.WindowSize(f); ok {
			p.tty = f
		}
	}
	p.color = p.tty != nil && os.Getenv("NO_COLOR") == ""
	p.mode = terminal.DetectColorMode()
	// Keep the LUT build out of the first Paint call
	if p.color && p.mode == terminal.ColorMode256 {
		terminal.WarmPalette256()
	}
	return p
}

// SetColor overrides style detection. Affects Paint only; live-block
// updates remain terminal-gated.
func (p *Printer) SetColor(on bool) {
	p.mu.Lock()
	p.color = on
	p.mu.Unlock()
}

// Size returns terminal dimensions, 80×24 when unknown
func (p *Printer) Size() (w, h int) {
	if p.tty != nil {
		if w, h, ok := terminal.WindowSize(p.tty); ok {
			return w, h
		}
	}
	return 80, 24
}

// Log prints a permanent line above the live block
func (p *Printer) Log(format string, a ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eraseLocked()
	fmt.Fprintf(p.w, format, a...)
	p.w.WriteByte('\n')
	p.redrawLocked()
	p.w.Flush()
}

// Update replaces the live block, rewriting in place. No-op on non-terminal output.
func (p *Printer) Update(lines ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tty == nil {
		return
	}
	p.eraseLocked()
	p.live = append(p.live[:0], lines...)
	p.redrawLocked()
	p.w.Flush()
}

// Done erases the live block and prints final permanent lines
func (p *Printer) Done(final ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eraseLocked()
	p.live = p.live[:0]
	for _, ln := range final {
		p.w.WriteString(ln)
		p.w.WriteByte('\n')
	}
	p.w.Flush()
}

// eraseLocked removes the drawn live block; cursor ends at block origin.
// Cursor sits one line below the block (redraw ends each line with '\n').
func (p *Printer) eraseLocked() {
	if p.tty == nil || p.drawn == 0 {
		return
	}
	fmt.Fprintf(p.w, "\x1b[%dA\r\x1b[J", p.drawn)
	p.drawn = 0
}

// redrawLocked writes the live block; assumes screen below cursor is clear.
// Lines are truncated to terminal width so cursor-up arithmetic stays valid
// (relies on xterm deferred autowrap for exact-width lines). Block is
// clamped to height-1 rows, keeping newest lines.
func (p *Printer) redrawLocked() {
	if p.tty == nil || len(p.live) == 0 {
		return
	}
	w, h := p.Size()
	lines := p.live
	if h > 1 && len(lines) > h-1 {
		lines = lines[len(lines)-(h-1):]
	}
	for _, ln := range lines {
		p.w.WriteString(truncVisible(ln, w))
		p.w.WriteByte('\n')
	}
	p.drawn = len(lines)
}
