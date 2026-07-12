package tui

import (
	"github.com/lixenwraith/terminal"
)

// ScrollBar draws vertical scrollbar track with thumb
func (r Region) ScrollBar(x int, offset, visible, total int, fg terminal.RGB) {
	if x < 0 || x >= r.W || r.H < 1 {
		return
	}

	trackH := r.H
	if total <= visible || trackH < 3 {
		// No scrolling needed or track too small
		for y := range trackH {
			r.Cell(x, y, '│', fg, terminal.RGB{}, terminal.AttrDim)
		}
		return
	}

	// Calculate thumb size and position
	thumbH := min(max((visible*trackH)/total, 1), trackH)

	maxScroll := total - visible
	thumbY := 0
	if maxScroll > 0 {
		thumbY = (offset * (trackH - thumbH)) / maxScroll
	}
	if thumbY < 0 {
		thumbY = 0
	}
	if thumbY+thumbH > trackH {
		thumbY = trackH - thumbH
	}

	// Draw track and thumb
	for y := range trackH {
		var ch rune
		if y >= thumbY && y < thumbY+thumbH {
			ch = '█'
		} else {
			ch = '░'
		}
		r.Cell(x, y, ch, fg, terminal.RGB{}, terminal.AttrNone)
	}
}

// ScrollIndicator draws compact indicator text (Top/Bot/XX%)
func (r Region) ScrollIndicator(y int, offset, visible, total int, fg terminal.RGB) {
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

	r.TextRight(y, text, fg, terminal.RGB{}, terminal.AttrDim)
}

