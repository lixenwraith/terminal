package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// LineType specifies box drawing character style
type LineType uint8

const (
	LineSingle  LineType = iota // ┌─┐│└┘
	LineDouble                  // ╔═╗║╚╝
	LineRounded                 // ╭─╮│╰╯
	LineHeavy                   // ┏━┓┃┗┛
	LineNone                    // spaces (invisible border with padding)
)

// boxChars contains box drawing character sets indexed by LineType
var boxChars = [...][6]rune{
	LineSingle:  {'┌', '─', '┐', '│', '└', '┘'},
	LineDouble:  {'╔', '═', '╗', '║', '╚', '╝'},
	LineRounded: {'╭', '─', '╮', '│', '╰', '╯'},
	LineHeavy:   {'┏', '━', '┓', '┃', '┗', '┛'},
	LineNone:    {' ', ' ', ' ', ' ', ' ', ' '},
}

const (
	boxTL = 0 // top-left
	boxH  = 1 // horizontal
	boxTR = 2 // top-right
	boxV  = 3 // vertical
	boxBL = 4 // bottom-left
	boxBR = 5 // bottom-right
)

// --- Box Rendering ---

// Box draws border around region edge
func (r Region) Box(line LineType, fg color.RGB) {
	r.BoxClipped(line, fg, r.H, 0)
}

// BoxClipped draws the visible slice of a box whose logical height is totalH,
// with off rows scrolled past its top edge. Rows outside the box are skipped,
// so a partially visible box keeps its sides and grows no false edges.
func (r Region) BoxClipped(line LineType, fg color.RGB, totalH, off int) {
	if r.W < 2 || r.H < 1 || totalH < 2 {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}

	chars := boxChars[line]
	bg := color.RGB{} // Transparent (use existing bg)

	for y := range r.H {
		c := off + y
		if c < 0 || c >= totalH {
			continue
		}
		switch c {
		case 0:
			r.Cell(0, y, chars[boxTL], fg, bg, terminal.AttrNone)
			r.Cell(r.W-1, y, chars[boxTR], fg, bg, terminal.AttrNone)
			for x := 1; x < r.W-1; x++ {
				r.Cell(x, y, chars[boxH], fg, bg, terminal.AttrNone)
			}
		case totalH - 1:
			r.Cell(0, y, chars[boxBL], fg, bg, terminal.AttrNone)
			r.Cell(r.W-1, y, chars[boxBR], fg, bg, terminal.AttrNone)
			for x := 1; x < r.W-1; x++ {
				r.Cell(x, y, chars[boxH], fg, bg, terminal.AttrNone)
			}
		default:
			r.Cell(0, y, chars[boxV], fg, bg, terminal.AttrNone)
			r.Cell(r.W-1, y, chars[boxV], fg, bg, terminal.AttrNone)
		}
	}
}

// BoxFilled draws border and fills interior with background
func (r Region) BoxFilled(line LineType, fg, bg color.RGB) {
	// Fill interior first
	for y := 1; y < r.H-1; y++ {
		for x := 1; x < r.W-1; x++ {
			r.Cell(x, y, ' ', fg, bg, terminal.AttrNone)
		}
	}
	// Draw border on top
	r.Box(line, fg)
}

// --- Line rendering ---

// HLine draws horizontal line across region width at row y
func (r Region) HLine(y int, line LineType, fg color.RGB) {
	if y < 0 || y >= r.H {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}
	ch := boxChars[line][boxH]
	for x := 0; x < r.W; x++ {
		r.Cell(x, y, ch, fg, color.RGB{}, terminal.AttrNone)
	}
}

// VLine draws vertical line across region height at column x
func (r Region) VLine(x int, line LineType, fg color.RGB) {
	if x < 0 || x >= r.W {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}
	ch := boxChars[line][boxV]
	for y := 0; y < r.H; y++ {
		r.Cell(x, y, ch, fg, color.RGB{}, terminal.AttrNone)
	}
}

// Divider draws horizontal line with optional centered label
func (r Region) Divider(y int, label string, line LineType, fg color.RGB) {
	if y < 0 || y >= r.H {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}

	hChar := boxChars[line][boxH]

	// Fill with horizontal line
	for x := 0; x < r.W; x++ {
		r.Cell(x, y, hChar, fg, color.RGB{}, terminal.AttrNone)
	}

	// Center label if provided
	if label != "" && r.W > 4 {
		text := " " + label + " "
		textLen := RuneLen(text)
		if textLen > r.W-2 {
			text = Truncate(text, r.W-2)
			textLen = RuneLen(text)
		}
		startX := (r.W - textLen) / 2
		for i, ch := range text {
			r.Cell(startX+i, y, ch, fg, color.RGB{}, terminal.AttrBold)
		}
	}
}

// --- Card rendering ---

// Card draws titled border and returns inner content region
func (r Region) Card(title string, line LineType, fg color.RGB) Region {
	r.Box(line, fg)

	if title != "" && r.W > 4 {
		maxTitleLen := r.W - 4
		displayTitle := title
		if RuneLen(displayTitle) > maxTitleLen {
			displayTitle = Truncate(displayTitle, maxTitleLen)
		}
		titleX := (r.W - RuneLen(displayTitle) - 2) / 2
		r.Text(titleX, 0, " "+displayTitle+" ", fg, color.RGB{}, terminal.AttrBold)
	}

	return r.Inset(1)
}
