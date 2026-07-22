package tui

import "github.com/lixenwraith/terminal"

// Align specifies text alignment within a column
type Align uint8

const (
	AlignLeft Align = iota
	AlignRight
	AlignCenter
)

// TableOpts configures table rendering
type TableOpts struct {
	ColWidths    []int   // Fixed widths per column, 0 = auto
	ColAligns    []Align // Alignment per column, default AlignLeft
	HeaderStyle  Style
	RowStyle     Style
	AltRowStyle  Style    // Alternating row style, zero = same as RowStyle
	ColSeparator rune     // Between columns, 0 = space
	RowSeparator LineType // Between rows, LineNone = no separator
}

// DefaultTableOpts returns sensible defaults
func DefaultTableOpts() TableOpts {
	return TableOpts{
		HeaderStyle:  Style{Attr: terminal.AttrBold},
		ColSeparator: ' ',
		RowSeparator: LineNone,
	}
}

// CalculateColumnWidths computes optimal column widths for given data
// Returns widths that fit within availableW, respecting fixed widths in opts
func CalculateColumnWidths(availableW int, headers []string, rows [][]string, opts TableOpts) []int {
	if len(headers) == 0 {
		return nil
	}

	cols := len(headers)
	widths := make([]int, cols)
	separatorW := 1 // space between columns

	// Start with header widths
	for i, h := range headers {
		widths[i] = RuneLen(h)
	}

	// Expand to fit data
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			w := RuneLen(row[i])
			if w > widths[i] {
				widths[i] = w
			}
		}
	}

	// Apply fixed widths from opts
	for i := 0; i < cols && i < len(opts.ColWidths); i++ {
		if opts.ColWidths[i] > 0 {
			widths[i] = opts.ColWidths[i]
		}
	}

	// Calculate total and scale if needed
	total := 0
	for _, w := range widths {
		total += w
	}
	total += (cols - 1) * separatorW

	if total > availableW && availableW > cols {
		// Proportionally shrink
		contentW := availableW - (cols-1)*separatorW
		scale := float64(contentW) / float64(total-(cols-1)*separatorW)
		for i := range widths {
			widths[i] = int(float64(widths[i]) * scale)
			if widths[i] < 1 {
				widths[i] = 1
			}
		}
	}

	return widths
}

// Table renders a table with headers and rows
func (r Region) Table(headers []string, rows [][]string, opts TableOpts) {
	if r.H < 1 || r.W < 1 || len(headers) == 0 {
		return
	}

	widths := CalculateColumnWidths(r.W, headers, rows, opts)
	sep := opts.ColSeparator
	if sep == 0 {
		sep = ' '
	}

	y := 0

	// Header row
	if y < r.H {
		r.renderTableRow(y, headers, widths, opts.ColAligns, sep, opts.HeaderStyle)
		y++
	}

	// Header separator
	if opts.RowSeparator != LineNone && y < r.H {
		r.HLine(y, opts.RowSeparator, opts.HeaderStyle.Fg)
		y++
	}

	// Data rows
	for rowIdx, row := range rows {
		if y >= r.H {
			break
		}

		style := opts.RowStyle
		if !opts.AltRowStyle.IsZero() && rowIdx%2 == 1 {
			style = opts.AltRowStyle
		}

		r.renderTableRow(y, row, widths, opts.ColAligns, sep, style)
		y++
	}
}

// renderTableRow renders a single table row
// The row is cleared to style.Bg across the full region width first, so
// alternating-row backgrounds render as continuous bands rather than only
// under glyphs. A zero style.Bg inherits the underlying background.
func (r Region) renderTableRow(y int, cells []string, widths []int, aligns []Align, sep rune, style Style) {
	// Clear row with row background (full width, List-consistent banding)
	for cx := 0; cx < r.W; cx++ {
		r.Cell(cx, y, ' ', style.Fg, style.Bg, terminal.AttrNone)
	}

	x := 0
	for i, w := range widths {
		if x >= r.W {
			break
		}

		text := ""
		if i < len(cells) {
			text = cells[i]
		}

		align := AlignLeft
		if i < len(aligns) {
			align = aligns[i]
		}

		// Truncate if needed
		if RuneLen(text) > w {
			text = Truncate(text, w)
		}

		// Render with alignment
		textLen := RuneLen(text)
		var startX int
		switch align {
		case AlignRight:
			startX = x + w - textLen
		case AlignCenter:
			startX = x + (w-textLen)/2
		default:
			startX = x
		}

		for j, ch := range text {
			if startX+j < r.W {
				r.Cell(startX+j, y, ch, style.Fg, style.Bg, style.Attr)
			}
		}

		x += w

		// Column separator
		if i < len(widths)-1 && x < r.W {
			r.Cell(x, y, sep, style.Fg, style.Bg, terminal.AttrDim)
			x++
		}
	}
}

