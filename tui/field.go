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
// focused. It returns the region for the value beside the label, where the
// caller draws a TextInput, Choice or Toggle, and the rows it used.
func (r Region) Field(y, labelW int, f Field, focused bool, th Theme) (Region, int) {
	label := th.Muted
	if focused {
		label = th.Text
		r.TextStyled(0, y, string(th.Glyphs.Focus), th.Accent)
	}
	name := f.Label
	if f.Required {
		name = Truncate(name, labelW-2)
		r.TextStyled(2+RuneLen(name)+1, y, string(th.Glyphs.Required), th.Accent)
	}
	r.TextStyled(2, y, Truncate(name, labelW), label)

	x := 2 + labelW + 1
	note, style := f.Error, th.Error
	if note == "" && focused {
		note, style = f.Help, th.Muted
	}
	if note == "" {
		return r.Sub(x, y, r.W-x, 1), 1
	}
	r.TextStyled(x, y+1, Truncate(note, r.W-x), style)
	return r.Sub(x, y, r.W-x, 1), 2
}

// TextInput draws state's text on r's first row, or, empty, the placeholder
// muted: a default shows what an empty value means. Focused, it draws on the
// Input background with the cursor, scrolling to keep the cursor in view.
func (r Region) TextInput(state *TextFieldState, placeholder string, focused bool, th Theme) {
	if r.W < 1 || r.H < 1 {
		return
	}
	var bg Style
	if focused {
		bg = th.Input
	}
	state.AdjustScroll(r.W)
	text, style := []rune(placeholder), th.Muted.On(bg)
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
