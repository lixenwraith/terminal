package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ScrollBarOpts configures scrollbar rendering
type ScrollBarOpts struct {
	ThumbFg  color.RGB
	TrackFg  color.RGB
	Bg       color.RGB // Zero inherits the existing cell background
	Thumb    rune      // 0 = '█'
	Track    rune      // 0 = '░'
	HideIdle bool      // Draw nothing when content fits the viewport
}

// ScrollBar draws a vertical scrollbar track with thumb in a single color
func (r Region) ScrollBar(x int, offset, visible, total int, fg color.RGB) {
	r.ScrollBarStyled(x, offset, visible, total, ScrollBarOpts{ThumbFg: fg, TrackFg: fg})
}

// ScrollBarStyled draws a vertical scrollbar at column x with explicit styling
func (r Region) ScrollBarStyled(x int, offset, visible, total int, opts ScrollBarOpts) {
	if x < 0 || x >= r.W || r.H < 1 {
		return
	}

	thumbCh, trackCh := opts.Thumb, opts.Track
	if thumbCh == 0 {
		thumbCh = '█'
	}
	if trackCh == 0 {
		trackCh = '░'
	}

	trackH := r.H
	if total <= visible || trackH < 3 {
		// No scrolling needed or track too small
		if opts.HideIdle {
			return
		}
		for y := range trackH {
			r.Cell(x, y, '│', opts.TrackFg, opts.Bg, terminal.AttrDim)
		}
		return
	}

	thumbH := min(max((visible*trackH)/total, 1), trackH)
	maxScroll := total - visible
	thumbY := 0
	if maxScroll > 0 {
		thumbY = (offset * (trackH - thumbH)) / maxScroll
	}
	thumbY = min(max(thumbY, 0), trackH-thumbH)

	for y := range trackH {
		if y >= thumbY && y < thumbY+thumbH {
			r.Cell(x, y, thumbCh, opts.ThumbFg, opts.Bg, terminal.AttrNone)
		} else {
			r.Cell(x, y, trackCh, opts.TrackFg, opts.Bg, terminal.AttrDim)
		}
	}
}

// ScrollIndicator draws compact indicator text (Top/Bot/XX%)
func (r Region) ScrollIndicator(y int, offset, visible, total int, fg color.RGB) {
	if y < 0 || y >= r.H {
		return
	}

	var text string
	if total <= visible || offset <= 0 {
		text = "Top"
	} else if offset+visible >= total {
		text = "Bot"
	} else {
		pct := ScrollPercent(offset, visible, total)
		if pct >= 100 {
			text = "99%"
		} else if pct >= 10 {
			text = string(rune('0'+pct/10)) + string(rune('0'+pct%10)) + "%"
		} else {
			text = " " + string(rune('0'+pct)) + "%"
		}
	}

	r.TextRight(y, text, fg, color.RGB{}, terminal.AttrDim)
}
