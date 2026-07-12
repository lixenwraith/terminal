package tui

import (
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
func (r Region) Box(line LineType, fg terminal.RGB) {
	if r.W < 2 || r.H < 2 {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}

	chars := boxChars[line]
	bg := terminal.RGB{} // Transparent (use existing bg)

	// Corners
	r.Cell(0, 0, chars[boxTL], fg, bg, terminal.AttrNone)
	r.Cell(r.W-1, 0, chars[boxTR], fg, bg, terminal.AttrNone)
	r.Cell(0, r.H-1, chars[boxBL], fg, bg, terminal.AttrNone)
	r.Cell(r.W-1, r.H-1, chars[boxBR], fg, bg, terminal.AttrNone)

	// Horizontal edges
	for x := 1; x < r.W-1; x++ {
		r.Cell(x, 0, chars[boxH], fg, bg, terminal.AttrNone)
		r.Cell(x, r.H-1, chars[boxH], fg, bg, terminal.AttrNone)
	}

	// Vertical edges
	for y := 1; y < r.H-1; y++ {
		r.Cell(0, y, chars[boxV], fg, bg, terminal.AttrNone)
		r.Cell(r.W-1, y, chars[boxV], fg, bg, terminal.AttrNone)
	}
}

// BoxFilled draws border and fills interior with background
func (r Region) BoxFilled(line LineType, fg, bg terminal.RGB) {
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
func (r Region) HLine(y int, line LineType, fg terminal.RGB) {
	if y < 0 || y >= r.H {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}
	ch := boxChars[line][boxH]
	for x := 0; x < r.W; x++ {
		r.Cell(x, y, ch, fg, terminal.RGB{}, terminal.AttrNone)
	}
}

// VLine draws vertical line across region height at column x
func (r Region) VLine(x int, line LineType, fg terminal.RGB) {
	if x < 0 || x >= r.W {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}
	ch := boxChars[line][boxV]
	for y := 0; y < r.H; y++ {
		r.Cell(x, y, ch, fg, terminal.RGB{}, terminal.AttrNone)
	}
}

// Divider draws horizontal line with optional centered label
func (r Region) Divider(y int, label string, line LineType, fg terminal.RGB) {
	if y < 0 || y >= r.H {
		return
	}
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}

	hChar := boxChars[line][boxH]

	// Fill with horizontal line
	for x := 0; x < r.W; x++ {
		r.Cell(x, y, hChar, fg, terminal.RGB{}, terminal.AttrNone)
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
			r.Cell(startX+i, y, ch, fg, terminal.RGB{}, terminal.AttrBold)
		}
	}
}

// --- Card rendering ---

// Card draws titled border and returns inner content region
func (r Region) Card(title string, line LineType, fg terminal.RGB) Region {
	r.Box(line, fg)

	if title != "" && r.W > 4 {
		maxTitleLen := r.W - 4
		displayTitle := title
		if RuneLen(displayTitle) > maxTitleLen {
			displayTitle = Truncate(displayTitle, maxTitleLen)
		}
		titleX := (r.W - RuneLen(displayTitle) - 2) / 2
		r.Text(titleX, 0, " "+displayTitle+" ", fg, terminal.RGB{}, terminal.AttrBold)
	}

	return r.Inset(1)
}