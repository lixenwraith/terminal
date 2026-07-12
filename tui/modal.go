package tui

import (
	"github.com/lixenwraith/terminal"
)

// ModalOpts configures modal overlay rendering
type ModalOpts struct {
	Title    string
	Hint     string // Right-aligned hint text
	Border   LineType
	BorderFg terminal.RGB
	TitleFg  terminal.RGB
	HintFg   terminal.RGB
	Bg       terminal.RGB
}

// Modal fills region with background, draws border with title/hint, returns content region
func (r Region) Modal(opts ModalOpts) Region {
	if r.W < 5 || r.H < 3 {
		return r.Sub(1, 1, 0, 0)
	}

	// Fill entire region
	r.Fill(opts.Bg)

	// Draw border
	r.Box(opts.Border, opts.BorderFg)

	// Title centered on top edge
	if opts.Title != "" {
		title := " " + opts.Title + " "
		titleLen := RuneLen(title)
		if titleLen > r.W-4 {
			title = Truncate(title, r.W-4)
			titleLen = RuneLen(title)
		}
		x := (r.W - titleLen) / 2
		for i, ch := range title {
			r.Cell(x+i, 0, ch, opts.TitleFg, opts.Bg, terminal.AttrBold)
		}
	}

	// Hint right-aligned on top edge
	if opts.Hint != "" {
		hint := opts.Hint
		hintLen := RuneLen(hint)
		if hintLen > r.W/3 {
			hint = Truncate(hint, r.W/3)
			hintLen = RuneLen(hint)
		}
		x := r.W - hintLen - 2
		if x < r.W/2 {
			x = r.W / 2
		}
		for i, ch := range hint {
			if x+i >= r.W-1 {
				break
			}
			r.Cell(x+i, 0, ch, opts.HintFg, opts.Bg, terminal.AttrNone)
		}
	}

	// Return content region
	return r.Sub(1, 1, r.W-2, r.H-2)
}