package tui

import "github.com/lixenwraith/terminal"

// Choice draws options in a row at (x, y), the chosen one bracketed in the
// accent style and the rest muted ("raw [txt] json"), and returns the
// columns it drew
func (r Region) Choice(x, y int, options []string, chosen int, th Theme) int {
	start := x
	for i, o := range options {
		open, close, style := " ", " ", th.Muted
		if i == chosen {
			open, close, style = "[", "]", th.Accent
		}
		r.TextStyled(x, y, open+o+close, style)
		x += RuneLen(o) + 2
	}
	return x - start
}

// StepChoice moves a choice among n options on Left or Right (h, l),
// stopping at either end, and reports whether it moved
func StepChoice(key terminal.Key, r rune, chosen, n int) (int, bool) {
	switch {
	case (key == terminal.KeyLeft || key == terminal.KeyRune && r == 'h') && chosen > 0:
		return chosen - 1, true
	case (key == terminal.KeyRight || key == terminal.KeyRune && r == 'l') && chosen < n-1:
		return chosen + 1, true
	}
	return chosen, false
}

// Toggle draws a switch at (x, y): the On glyph and "on" in the accent
// style, or the Off glyph and "off" muted, and returns the columns it drew
func (r Region) Toggle(x, y int, on bool, th Theme) int {
	label, style := string(th.Glyphs.Off)+" off", th.Muted
	if on {
		label, style = string(th.Glyphs.On)+" on", th.Accent
	}
	r.TextStyled(x, y, label, style)
	return RuneLen(label)
}
