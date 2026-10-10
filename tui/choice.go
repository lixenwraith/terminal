package tui

import "github.com/lixenwraith/terminal"

// Radio draws options as radio buttons in a row at (x, y): the chosen one
// with the On glyph in the accent style, the rest with Off, muted ("○ raw
// ● txt  ○ json"); chosen -1 marks none. Where the row does not fit, it shows
// the options around the chosen one, a muted '…' where others are hidden.
// It returns the columns it drew.
func (r Region) Radio(x, y int, options []string, chosen int, th Theme) int {
	width := func(lo, hi int) int { // the window [lo, hi), its '…' marks included
		w := -2
		for _, o := range options[lo:hi] {
			w += RuneLen(o) + 4
		}
		return w + 2*min(lo, 1) + 3*min(len(options)-hi, 1)
	}
	lo, hi := 0, len(options)
	if width(lo, hi) > r.W-x && hi > 0 {
		lo = max(0, min(chosen, hi-1))
		hi = lo + 1
		for grown := true; grown; {
			grown = false
			if hi < len(options) && width(lo, hi+1) <= r.W-x {
				hi, grown = hi+1, true
			}
			if lo > 0 && width(lo-1, hi) <= r.W-x {
				lo, grown = lo-1, true
			}
		}
	}
	start := x
	if lo > 0 {
		r.TextStyled(x, y, "…", th.Muted)
		x += 2
	}
	for i := lo; i < hi; i++ {
		mark, style := th.Glyphs.Off, th.Muted
		if i == chosen {
			mark, style = th.Glyphs.On, th.Accent
		}
		r.TextStyled(x, y, Truncate(string(mark)+" "+options[i], max(0, r.W-x)), style)
		x += RuneLen(options[i]) + 4
	}
	if hi < len(options) {
		r.TextStyled(x, y, "…", th.Muted)
		x += 3
	}
	return max(0, min(x-2, r.W)-start)
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
