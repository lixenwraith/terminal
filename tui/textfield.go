package tui

import (
	"github.com/lixenwraith/terminal"
)

// TextFieldOpts configures text field rendering
type TextFieldOpts struct {
	Placeholder string   // Shown when empty
	Prefix      string   // Left prompt (e.g., "> ")
	Mask        rune     // Password mask, 0 = none
	MaxLen      int      // Max runes, 0 = unlimited
	Border      LineType // Border style, LineNone = no border
	Focused     bool     // Show cursor and accept input
	Style       TextFieldStyle
}

// DefaultTextFieldStyle returns default colors
func DefaultTextFieldStyle() TextFieldStyle {
	return TextFieldStyle{
		TextFg:        terminal.RGB{R: 220, G: 220, B: 220},
		TextBg:        terminal.RGB{R: 30, G: 30, B: 40},
		CursorFg:      terminal.RGB{R: 0, G: 0, B: 0},
		CursorBg:      terminal.RGB{R: 200, G: 200, B: 200},
		PlaceholderFg: terminal.RGB{R: 100, G: 100, B: 110},
		PrefixFg:      terminal.RGB{R: 150, G: 150, B: 180},
		BorderFg:      terminal.RGB{R: 80, G: 80, B: 100},
	}
}

// TextFieldStyle defines text field colors
type TextFieldStyle struct {
	TextFg        terminal.RGB
	TextBg        terminal.RGB
	CursorFg      terminal.RGB
	CursorBg      terminal.RGB
	PlaceholderFg terminal.RGB
	PrefixFg      terminal.RGB
	BorderFg      terminal.RGB
}

// TextField renders text field and returns content height used
func (r Region) TextField(state *TextFieldState, opts TextFieldOpts) int {
	if r.W < 3 || r.H < 1 {
		return 0
	}

	style := opts.Style
	if style == (TextFieldStyle{}) {
		style = DefaultTextFieldStyle()
	}

	// Calculate content area
	contentY := 0
	contentX := 0
	contentW := r.W
	contentH := 1

	if opts.Border != LineNone {
		if r.H < 3 {
			return 0
		}
		r.Box(opts.Border, style.BorderFg)
		contentY = 1
		contentX = 1
		contentW = r.W - 2
		contentH = r.H - 2
		if contentH > 1 {
			contentH = 1
		}
	}

	// Fill background
	for x := contentX; x < contentX+contentW; x++ {
		r.Cell(x, contentY, ' ', style.TextFg, style.TextBg, terminal.AttrNone)
	}

	x := contentX

	// Prefix
	if opts.Prefix != "" {
		for _, ch := range opts.Prefix {
			if x >= contentX+contentW {
				break
			}
			r.Cell(x, contentY, ch, style.PrefixFg, style.TextBg, terminal.AttrNone)
			x++
		}
	}

	// Calculate viewport
	viewportW := contentX + contentW - x
	if viewportW < 1 {
		return contentH + 2*boolToInt(opts.Border != LineNone)
	}

	// Adjust scroll
	state.AdjustScroll(viewportW)

	// Render text or placeholder
	text := state.Text
	isEmpty := len(text) == 0

	if isEmpty && opts.Placeholder != "" && !opts.Focused {
		// Placeholder
		placeholder := opts.Placeholder
		if RuneLen(placeholder) > viewportW {
			placeholder = Truncate(placeholder, viewportW)
		}
		for i, ch := range placeholder {
			if x+i >= contentX+contentW {
				break
			}
			r.Cell(x+i, contentY, ch, style.PlaceholderFg, style.TextBg, terminal.AttrDim)
		}
	} else {
		// Scroll indicators
		if state.Scroll > 0 && x > contentX {
			r.Cell(x-1, contentY, '◀', style.PlaceholderFg, style.TextBg, terminal.AttrNone)
		}

		// Text content
		for i := 0; i < viewportW; i++ {
			runeIdx := state.Scroll + i
			ch := ' '
			if runeIdx < len(text) {
				ch = text[runeIdx]
				if opts.Mask != 0 {
					ch = opts.Mask
				}
			}

			fg := style.TextFg
			bg := style.TextBg

			// Cursor highlighting
			if opts.Focused && runeIdx == state.Cursor {
				fg = style.CursorFg
				bg = style.CursorBg
			}

			r.Cell(x+i, contentY, ch, fg, bg, terminal.AttrNone)
		}

		// Cursor at end
		if opts.Focused && state.Cursor == len(text) {
			cursorX := x + state.Cursor - state.Scroll
			if cursorX >= x && cursorX < contentX+contentW {
				r.Cell(cursorX, contentY, ' ', style.CursorFg, style.CursorBg, terminal.AttrNone)
			}
		}

		// Right scroll indicator
		if state.Scroll+viewportW < len(text) {
			r.Cell(contentX+contentW-1, contentY, '▶', style.PlaceholderFg, style.TextBg, terminal.AttrNone)
		}
	}

	if opts.Border != LineNone {
		return 3
	}
	return 1
}

// boolToInt converts boolean to integer (0 or 1)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}