package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// PaneOpts configures pane rendering
type PaneOpts struct {
	Title    string
	Border   LineType
	BorderFg color.RGB
	Bg       color.RGB
	TitleFg  color.RGB
}

// Pane draws bordered pane with optional title, returns content region
// Content region is inside border, below title row if present
func (r Region) Pane(opts PaneOpts) Region {
	if r.W < 3 || r.H < 3 {
		return r.Sub(1, 1, 0, 0)
	}

	// Fill background
	r.Fill(opts.Bg)

	// Draw border
	r.Box(opts.Border, opts.BorderFg)

	// Title on top edge
	headerH := 0
	if opts.Title != "" {
		headerH = 1
		title := " " + opts.Title + " "
		if RuneLen(title) > r.W-4 {
			title = Truncate(title, r.W-4)
		}
		x := 2
		for i, ch := range title {
			if x+i >= r.W-1 {
				break
			}
			r.Cell(x+i, 0, ch, opts.TitleFg, opts.Bg, terminal.AttrBold)
		}
	}

	// Return content region (inside border, below title)
	return r.Sub(1, 1+headerH, r.W-2, r.H-2-headerH)
}

// TitledPane fills region with background, draws centered title at top, returns content region
// Content region starts at row 1 with full width
func (r Region) TitledPane(title string, titleFg, bg color.RGB) Region {
	r.Fill(bg)
	if title != "" && r.H > 0 {
		r.TextCenter(0, title, titleFg, bg, terminal.AttrBold)
	}
	if r.H <= 1 {
		return r.Sub(0, 0, r.W, 0)
	}
	return r.Sub(0, 1, r.W, r.H-1)
}

// TitledPaneFocused is TitledPane with focus-dependent background
func (r Region) TitledPaneFocused(title string, titleFg, bg, focusBg color.RGB, focused bool) Region {
	if focused {
		bg = focusBg
	}
	return r.TitledPane(title, titleFg, bg)
}

