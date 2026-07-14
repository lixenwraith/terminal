package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Default spinner frames for Region.Spinner
var spinnerFrames = spinnerSets[SpinnerBraille]

// Text renders text at position, truncates at region edge
func (r Region) Text(x, y int, s string, fg, bg color.RGB, attr terminal.Attr) {
	if y < 0 || y >= r.H {
		return
	}
	col := 0
	for _, ch := range s {
		if x+col >= r.W {
			break
		}
		if x+col >= 0 {
			r.Cell(x+col, y, ch, fg, bg, attr)
		}
		col++
	}
}

// TextStyled renders text using Style struct
func (r Region) TextStyled(x, y int, s string, style Style) {
	if y < 0 || y >= r.H {
		return
	}
	col := 0
	for _, ch := range s {
		if x+col >= r.W {
			break
		}
		if x+col >= 0 {
			r.Cell(x+col, y, ch, style.Fg, style.Bg, style.Attr)
		}
		col++
	}
}

// TextRight renders text right-aligned on row
func (r Region) TextRight(y int, s string, fg, bg color.RGB, attr terminal.Attr) {
	x := r.W - RuneLen(s)
	r.Text(x, y, s, fg, bg, attr)
}

// TextCenter renders text centered on row
func (r Region) TextCenter(y int, s string, fg, bg color.RGB, attr terminal.Attr) {
	x := (r.W - RuneLen(s)) / 2
	r.Text(x, y, s, fg, bg, attr)
}

// TextBlock renders wrapped text within region bounds, returns number of lines rendered
func (r Region) TextBlock(x, y int, text string, fg, bg color.RGB, attr terminal.Attr) int {
	if x >= r.W || y >= r.H || text == "" {
		return 0
	}

	availW := r.W - x
	if availW < 1 {
		return 0
	}

	lines := WrapText(text, availW)
	rendered := 0

	for i, line := range lines {
		lineY := y + i
		if lineY >= r.H {
			break
		}
		r.Text(x, lineY, line, fg, bg, attr)
		rendered++
	}

	return rendered
}

// TextBlockStyled renders wrapped text using Style struct, returns number of lines rendered
func (r Region) TextBlockStyled(x, y int, text string, style Style) int {
	return r.TextBlock(x, y, text, style.Fg, style.Bg, style.Attr)
}
