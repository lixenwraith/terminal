package tui

import (
	"github.com/lixenwraith/terminal"
)

// Focus is a focus ring over a form's controls, numbered 0 to Len-1
type Focus struct {
	Index, Len int
}

// HandleKey moves focus on Tab and Shift+Tab, wrapping, and reports whether
// it moved
func (f *Focus) HandleKey(key terminal.Key, mod terminal.Modifier) bool {
	if f.Len < 1 {
		return false
	}
	switch {
	case key == terminal.KeyTab && mod&terminal.ModShift == 0:
		f.Index = (f.Index + 1) % f.Len
	case key == terminal.KeyTab || key == terminal.KeyBacktab:
		f.Index = (f.Index - 1 + f.Len) % f.Len
	default:
		return false
	}
	return true
}

// Field labels one form row
type Field struct {
	Label    string
	Required bool
	Help     string // shown under the field while it has focus
	Error    string // shown under the field, in place of Help
}

// Field draws a form row at y: the focus mark, the label in a column labelW
// wide with the required mark, and below, the error, or the help while
// focused, wrapped. The label column leaves the value half the row; labelW 0
// stacks the label on its own row and the value under it, indented. It
// returns the region for the value, where the caller draws a TextInput,
// Radio or Toggle, and the rows it used.
func (r Region) Field(y, labelW int, f Field, focused bool, th Theme) (Region, int) {
	label := th.Muted
	if focused {
		label = th.Text
		r.TextStyled(0, y, string(th.Glyphs.Focus), th.Accent)
	}
	x, rows := 4, 2 // stacked: the label's own row, the value under it
	if labelW > 0 {
		labelW = min(labelW, max(1, (r.W-3)/2))
		x, rows = 2+labelW+1, 1
	} else {
		labelW = r.W - 2
	}
	name := f.Label
	if f.Required {
		name = Truncate(name, labelW-2)
		r.TextStyled(2+RuneLen(name)+1, y, string(th.Glyphs.Required), th.Accent)
	}
	r.TextStyled(2, y, Truncate(name, labelW), label)
	value := r.Sub(x, y+rows-1, r.W-x, 1)

	note, style := f.Error, th.Error
	if note == "" && focused {
		note, style = f.Help, th.Muted
	}
	if note == "" {
		return value, rows
	}
	lines := WrapText(note, r.W-x)
	for i, l := range lines {
		r.TextStyled(x, y+rows+i, l, style)
	}
	return value, rows + len(lines)
}

// TextInput draws state's text on r's first row, or, empty, the placeholder
// muted: a default shows what an empty value means. Focused, it draws on the
// Input background with the cursor, scrolling to keep the cursor in view; a
// muted '…' marks text out of view.
func (r Region) TextInput(state *TextFieldState, placeholder string, focused bool, th Theme) {
	if r.W < 1 || r.H < 1 {
		return
	}
	var bg Style
	if focused {
		bg = th.Input
	}
	state.AdjustScroll(r.W)
	text, style := []rune(Truncate(placeholder, r.W)), th.Muted.On(bg)
	if len(state.Text) > 0 {
		text, style = state.Text[state.Scroll:], th.Text.On(bg)
	}
	for x := 0; x < r.W; x++ {
		ch := ' '
		if x < len(text) {
			ch = text[x]
		}
		r.Cell(x, 0, ch, style.Fg, style.Bg, style.Attr)
	}
	// Text scrolled out of view on either side is marked, never under the cursor
	hidden := th.Muted.On(bg)
	if state.Scroll > 0 && (state.Cursor != state.Scroll || !focused) {
		r.Cell(0, 0, '…', hidden.Fg, hidden.Bg, hidden.Attr)
	}
	if end := r.W - 1; len(text) > r.W && (state.Cursor-state.Scroll != end || !focused) {
		r.Cell(end, 0, '…', hidden.Fg, hidden.Bg, hidden.Attr)
	}
	if focused {
		x, ch := state.Cursor-state.Scroll, ' '
		if x < len(text) {
			ch = text[x]
		}
		r.Cell(x, 0, ch, th.Cursor.Fg, th.Cursor.Bg, th.Cursor.Attr)
	}
}

// Group draws a foldable group's header at y: the focus mark, the Folded or
// Unfolded glyph and the title, and while folded, a muted summary of what
// it holds
func (r Region) Group(y int, title, summary string, open, focused bool, th Theme) {
	mark, style := th.Glyphs.Folded, th.Muted
	if open {
		mark = th.Glyphs.Unfolded
	}
	if focused {
		style = th.Text
		r.TextStyled(0, y, string(th.Glyphs.Focus), th.Accent)
	}
	r.TextStyled(2, y, string(mark)+" "+title, style)
	if !open && summary != "" {
		x := 2 + 2 + RuneLen(title) + 2
		r.TextStyled(x, y, Truncate(summary, r.W-x), th.Muted)
	}
}
