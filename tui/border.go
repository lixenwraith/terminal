package tui

import (
	"strings"

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
	LineASCII                   // +-+|++, for a terminal outside UTF-8
	LineDashed                  // ╭╌╮╎╰╯
)

// lineChars are each LineType's runes by the arms that meet in a cell: up 1,
// right 2, down 4, left 8, so corners, tees and crossings all join
var lineChars = [...]string{
	LineSingle:  " │─└││┌├─┘─┴┐┤┬┼",
	LineDouble:  " ║═╚║║╔╠═╝═╩╗╣╦╬",
	LineRounded: " │─╰││╭├─╯─┴╮┤┬┼",
	LineHeavy:   " ┃━┗┃┃┏┣━┛━┻┓┫┳╋",
	LineNone:    "                ",
	LineASCII:   " |-+||++-+-+++++",
	LineDashed:  " ╎╌╰╎╎╭├╌╯╌┴╮┤┬┼",
}

const (
	boxTL = 6  // top-left
	boxH  = 10 // horizontal
	boxTR = 12 // top-right
	boxV  = 5  // vertical
	boxBL = 3  // bottom-left
	boxBR = 9  // bottom-right
)

// runes returns the line's runes by arms; an unknown type draws single
func (l LineType) runes() []rune {
	if l >= LineType(len(lineChars)) {
		l = LineSingle
	}
	return []rune(lineChars[l])
}

// --- Box Rendering ---

// Box draws border around region edge
func (r Region) Box(line LineType, fg color.RGB) {
	r.BoxClipped(line, fg, r.H, 0)
}

// BoxClipped draws the visible slice of a box whose logical height is totalH,
// with off rows scrolled past its top edge. Rows outside the box are skipped,
// so a partially visible box keeps its sides and grows no false edges.
func (r Region) BoxClipped(line LineType, fg color.RGB, totalH, off int) {
	r.boxStyled(line, Style{Fg: fg}, totalH, off)
}

// BoxStyle draws a border around the region's edge in a theme's style
func (r Region) BoxStyle(line LineType, s Style) {
	r.boxStyled(line, s, r.H, 0)
}

func (r Region) boxStyled(line LineType, s Style, totalH, off int) {
	if r.W < 2 || r.H < 1 || totalH < 2 {
		return
	}
	chars := line.runes()
	edge := func(y int, left, right rune) {
		r.TextStyled(0, y, string(left), s)
		r.TextStyled(r.W-1, y, string(right), s)
	}
	for y := range r.H {
		switch c := off + y; {
		case c < 0 || c >= totalH:
		case c == 0 || c == totalH-1:
			left, right := chars[boxTL], chars[boxTR]
			if c > 0 {
				left, right = chars[boxBL], chars[boxBR]
			}
			r.TextStyled(1, y, strings.Repeat(string(chars[boxH]), max(0, r.W-2)), s)
			edge(y, left, right)
		default:
			edge(y, chars[boxV], chars[boxV])
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
	ch := line.runes()[boxH]
	for x := 0; x < r.W; x++ {
		r.Cell(x, y, ch, fg, color.RGB{}, terminal.AttrNone)
	}
}

// VLine draws vertical line across region height at column x
func (r Region) VLine(x int, line LineType, fg color.RGB) {
	if x < 0 || x >= r.W {
		return
	}
	ch := line.runes()[boxV]
	for y := 0; y < r.H; y++ {
		r.Cell(x, y, ch, fg, color.RGB{}, terminal.AttrNone)
	}
}

// Divider draws horizontal line with optional centered label
func (r Region) Divider(y int, label string, line LineType, fg color.RGB) {
	if y < 0 || y >= r.H {
		return
	}
	hChar := line.runes()[boxH]

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

// --- Themed frames, rules and wires ---

// Frame fills the region with the theme's text style and draws its border
// in Glyphs.Line, the title on the top edge, and returns the inside with a
// column of padding each side. Callers size it to its content, with Center.
func (r Region) Frame(title string, th Theme) Region {
	r.FillStyle(th.Text)
	r.BoxStyle(th.Glyphs.Line, th.Border)
	if title != "" && r.W > 6 {
		r.TextStyled(2, 0, " "+Truncate(title, r.W-6)+" ", th.Accent)
	}
	return r.Sub(2, 1, r.W-4, r.H-2)
}

// Rule draws a line across row y, the title at its third column and the
// hint muted at its end. The title keeps its room: the hint is dropped where
// both do not fit.
func (r Region) Rule(y int, title, hint string, th Theme) {
	r.TextStyled(0, y, strings.Repeat(string(th.Glyphs.Line.runes()[boxH]), max(0, r.W)), th.Border)
	if title != "" && r.W > 6 {
		title = " " + Truncate(title, r.W-6) + " "
		r.TextStyled(2, y, title, th.Text)
	}
	if hint != "" && RuneLen(title)+RuneLen(hint)+6 <= r.W {
		r.TextStyled(r.W-RuneLen(hint)-3, y, " "+hint+" ", th.Muted)
	}
}

// Wires are lines on a grid, each cell holding the arms that meet there: up
// 1, right 2, down 4, left 8. Wherever lines branch or cross, the cell draws
// the junction, whatever order they were laid in.
type Wires struct {
	W    int
	Arms []uint8
}

// NewWires creates an empty grid w by h
func NewWires(w, h int) Wires {
	return Wires{W: w, Arms: make([]uint8, max(0, w*h))}
}

// H lays a wire along row y between columns x0 and x1
func (g Wires) H(y, x0, x1 int) {
	for x := min(x0, x1); x < max(x0, x1); x++ {
		g.arm(x, y, 2)
		g.arm(x+1, y, 8)
	}
}

// V lays a wire down column x between rows y0 and y1
func (g Wires) V(x, y0, y1 int) {
	for y := min(y0, y1); y < max(y0, y1); y++ {
		g.arm(x, y, 4)
		g.arm(x, y+1, 1)
	}
}

// arm adds an arm to a cell; one off the grid is dropped
func (g Wires) arm(x, y int, a uint8) {
	if i := y*g.W + x; x >= 0 && x < g.W && y >= 0 && i < len(g.Arms) {
		g.Arms[i] |= a
	}
}

// DrawWires draws every cell of the grid that has arms, in the line type
func (r Region) DrawWires(g Wires, line LineType, s Style) {
	chars := line.runes()
	for i, arms := range g.Arms {
		if arms != 0 {
			r.TextStyled(i%g.W, i/g.W, string(chars[arms&15]), s)
		}
	}
}
