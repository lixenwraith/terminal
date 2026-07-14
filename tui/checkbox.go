package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// CheckState represents checkbox visual state
type CheckState uint8

const (
	CheckNone    CheckState = iota // [ ]
	CheckPartial                   // [o]
	CheckFull                      // [x]
	CheckPlus                      // [+]
)

// Checkbox draws a checkbox indicator
func (r Region) Checkbox(x, y int, state CheckState, fg color.RGB) {
	if x < 0 || x+2 >= r.W || y < 0 || y >= r.H {
		return
	}
	var ch rune
	switch state {
	case CheckNone:
		ch = ' '
	case CheckPartial:
		ch = 'o'
	case CheckFull:
		ch = 'x'
	case CheckPlus:
		ch = '+'
	}
	r.Cell(x, y, '[', fg, color.RGB{}, terminal.AttrNone)
	r.Cell(x+1, y, ch, fg, color.RGB{}, terminal.AttrNone)
	r.Cell(x+2, y, ']', fg, color.RGB{}, terminal.AttrNone)
}

