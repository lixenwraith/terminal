package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ListItem represents a single row in a scrollable list
type ListItem struct {
	Indent    int  // Left padding in cells
	Icon      rune // Expand indicator or bullet, 0 = none
	IconFg    color.RGB
	Check     CheckState // CheckNone to skip checkbox
	CheckFg   color.RGB
	Text      string
	TextStyle Style
}

// ListOpts configures list rendering
type ListOpts struct {
	CursorBg  color.RGB
	DefaultBg color.RGB
	IconWidth int // Width reserved for icon, default 2
}

// List renders scrollable list items within region, returns number of rows rendered
func (r Region) List(items []ListItem, cursor, scroll int, opts ListOpts) int {
	if r.H < 1 || len(items) == 0 {
		return 0
	}

	iconW := opts.IconWidth
	if iconW == 0 {
		iconW = 2
	}

	rendered := 0
	for y := 0; y < r.H; y++ {
		idx := scroll + y
		if idx >= len(items) {
			break
		}

		item := items[idx]
		isCursor := idx == cursor

		// Row background
		bg := opts.DefaultBg
		if isCursor {
			bg = opts.CursorBg
		}

		// Clear row
		for x := 0; x < r.W; x++ {
			r.Cell(x, y, ' ', color.RGB{}, bg, terminal.AttrNone)
		}

		x := item.Indent

		// Icon
		if item.Icon != 0 && x < r.W {
			r.Cell(x, y, item.Icon, item.IconFg, bg, terminal.AttrNone)
		}
		x += iconW

		// Checkbox
		if item.Check != CheckNone || item.CheckFg != (color.RGB{}) {
			if x+3 <= r.W {
				var ch rune
				switch item.Check {
				case CheckNone:
					ch = ' '
				case CheckPartial:
					ch = 'o'
				case CheckFull:
					ch = 'x'
				case CheckPlus:
					ch = '+'
				}
				r.Cell(x, y, '[', item.CheckFg, bg, terminal.AttrNone)
				r.Cell(x+1, y, ch, item.CheckFg, bg, terminal.AttrNone)
				r.Cell(x+2, y, ']', item.CheckFg, bg, terminal.AttrNone)
			}
			x += 4
		}

		// Text
		textStyle := item.TextStyle
		if textStyle.Bg == (color.RGB{}) {
			textStyle.Bg = bg
		}
		text := item.Text
		if x+RuneLen(text) > r.W {
			text = Truncate(text, r.W-x)
		}
		r.TextStyled(x, y, text, textStyle)

		rendered++
	}

	return rendered
}

