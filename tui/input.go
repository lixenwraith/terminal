package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// InputOpts configures single-line input field
type InputOpts struct {
	Label    string
	LabelFg  color.RGB
	Text     string
	Cursor   int // Cursor position in text (rune index)
	CursorBg color.RGB
	TextFg   color.RGB
	Bg       color.RGB
}

// Input renders labeled text input field on row y, handling cursor display and horizontal scrolling
func (r Region) Input(y int, opts InputOpts) {
	if y < 0 || y >= r.H || r.W < 5 {
		return
	}

	x := 0

	// Label
	if opts.Label != "" {
		for _, ch := range opts.Label {
			if x >= r.W {
				break
			}
			r.Cell(x, y, ch, opts.LabelFg, opts.Bg, terminal.AttrNone)
			x++
		}
	}

	// Available width for input text
	inputW := r.W - x
	if inputW < 3 {
		return
	}

	runes := []rune(opts.Text)
	cursor := opts.Cursor
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if cursor < 0 {
		cursor = 0
	}

	// Horizontal scroll to keep cursor visible
	scroll := 0
	if cursor >= inputW-1 {
		scroll = cursor - inputW + 2
	}

	// Render visible portion
	for i := 0; i < inputW; i++ {
		runeIdx := scroll + i
		ch := ' '
		if runeIdx < len(runes) {
			ch = runes[runeIdx]
		}

		bg := opts.Bg
		if runeIdx == cursor {
			bg = opts.CursorBg
		}

		r.Cell(x+i, y, ch, opts.TextFg, bg, terminal.AttrNone)
	}

	// Cursor at end if past text
	if cursor == len(runes) && cursor-scroll < inputW {
		r.Cell(x+cursor-scroll, y, ' ', opts.TextFg, opts.CursorBg, terminal.AttrNone)
	}
}

