package tui

import (
	"github.com/lixenwraith/terminal"
)

// Progress bar characters
const (
	progressFull  = '█'
	progressEmpty = '░'
	progressHalf  = '▌'
)

// Progress draws horizontal progress bar (0.0-1.0)
func (r Region) Progress(x, y, w int, pct float64, fg, bg terminal.RGB) {
	if y < 0 || y >= r.H || w <= 0 {
		return
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	filled := int(float64(w) * pct)
	remainder := float64(w)*pct - float64(filled)

	for i := 0; i < w; i++ {
		if x+i >= r.W {
			break
		}
		var ch rune
		if i < filled {
			ch = progressFull
		} else if i == filled && remainder >= 0.5 {
			ch = progressHalf
		} else {
			ch = progressEmpty
		}
		r.Cell(x+i, y, ch, fg, bg, terminal.AttrNone)
	}
}

// ProgressV draws vertical progress bar (fills bottom-up)
func (r Region) ProgressV(x, y, h int, pct float64, fg, bg terminal.RGB) {
	if x < 0 || x >= r.W || h <= 0 {
		return
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	filled := int(float64(h) * pct)

	for i := 0; i < h; i++ {
		if y+i >= r.H {
			break
		}
		var ch rune
		// Fill from bottom up
		if h-1-i < filled {
			ch = progressFull
		} else {
			ch = progressEmpty
		}
		r.Cell(x, y+i, ch, fg, bg, terminal.AttrNone)
	}
}

// Spinner draws spinner character based on frame counter
func (r Region) Spinner(x, y int, frame int, fg terminal.RGB) {
	if x < 0 || x >= r.W || y < 0 || y >= r.H {
		return
	}
	idx := frame % len(spinnerFrames)
	if idx < 0 {
		idx = -idx
	}
	r.Cell(x, y, spinnerFrames[idx], fg, terminal.RGB{}, terminal.AttrNone)
}

// Gauge draws labeled gauge with percentage
func (r Region) Gauge(x, y, w int, value, max int, fg, bg terminal.RGB) {
	if w < 5 || y < 0 || y >= r.H {
		return
	}

	var pct float64
	if max > 0 {
		pct = float64(value) / float64(max)
	}
	if pct > 1 {
		pct = 1
	}
	if pct < 0 {
		pct = 0
	}

	// Format: [████░░░░] 75%
	labelW := 5 // " XXX%" or " 100%"
	barW := w - labelW - 2
	if barW < 1 {
		barW = 1
	}

	r.Cell(x, y, '[', fg, bg, terminal.AttrNone)
	r.Progress(x+1, y, barW, pct, fg, bg)
	r.Cell(x+1+barW, y, ']', fg, bg, terminal.AttrNone)

	pctInt := int(pct * 100)
	var label string
	if pctInt >= 100 {
		label = " 100%"
	} else if pctInt >= 10 {
		label = " " + string(rune('0'+pctInt/10)) + string(rune('0'+pctInt%10)) + "%"
	} else {
		label = "  " + string(rune('0'+pctInt)) + "%"
	}
	r.Text(x+2+barW, y, label, fg, bg, terminal.AttrNone)
}