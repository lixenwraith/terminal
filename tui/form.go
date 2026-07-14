package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// FormField pairs a label with an editable text field
type FormField struct {
	Label string
	State *TextFieldState
}

// FormState holds state for a multi-field form with focus tracking
type FormState struct {
	Fields []FormField
	Focus  int
}

// NewFormState creates a form with labeled fields initialized to empty values
func NewFormState(labels ...string) *FormState {
	fields := make([]FormField, len(labels))
	for i, label := range labels {
		fields[i] = FormField{
			Label: label,
			State: NewTextFieldState(""),
		}
	}
	return &FormState{Fields: fields}
}

// Value returns the text value of the field at idx
func (f *FormState) Value(idx int) string {
	if idx >= 0 && idx < len(f.Fields) {
		return f.Fields[idx].State.Value()
	}
	return ""
}

// SetValue replaces the text of the field at idx
func (f *FormState) SetValue(idx int, val string) {
	if idx >= 0 && idx < len(f.Fields) {
		f.Fields[idx].State.SetValue(val)
	}
}

// Clear empties all fields in the form
func (f *FormState) Clear() {
	for i := range f.Fields {
		f.Fields[i].State.Clear()
	}
}

// FocusNext moves focus to the next field, wrapping around
func (f *FormState) FocusNext() {
	if len(f.Fields) > 0 {
		f.Focus = (f.Focus + 1) % len(f.Fields)
	}
}

// FocusPrev moves focus to the previous field, wrapping around
func (f *FormState) FocusPrev() {
	if len(f.Fields) > 0 {
		f.Focus = (f.Focus - 1 + len(f.Fields)) % len(f.Fields)
	}
}

// CurrentField returns the TextFieldState of the focused field, or nil
func (f *FormState) CurrentField() *TextFieldState {
	if f.Focus >= 0 && f.Focus < len(f.Fields) {
		return f.Fields[f.Focus].State
	}
	return nil
}

// HandleKey processes keyboard input for form navigation and field editing, returns true if state changed
func (f *FormState) HandleKey(key terminal.Key, r rune, mod terminal.Modifier) bool {
	switch key {
	case terminal.KeyTab:
		if mod&terminal.ModShift != 0 {
			f.FocusPrev()
		} else {
			f.FocusNext()
		}
		return true
	case terminal.KeyUp:
		f.FocusPrev()
		return true
	case terminal.KeyDown:
		f.FocusNext()
		return true
	default:
		if field := f.CurrentField(); field != nil {
			return field.HandleKey(key, r, mod)
		}
	}
	return false
}

// FormOpts configures form rendering
type FormOpts struct {
	LabelWidth int
	Spacing    int
	Style      FormStyle
}

// FormStyle defines form colors
type FormStyle struct {
	LabelFg  color.RGB
	FieldFg  color.RGB
	FieldBg  color.RGB
	FocusBg  color.RGB
	CursorFg color.RGB
	CursorBg color.RGB
	Bg       color.RGB
}

// DefaultFormStyle returns default form colors
func DefaultFormStyle() FormStyle {
	return FormStyle{
		LabelFg:  color.RGB{R: 150, G: 150, B: 180},
		FieldFg:  color.RGB{R: 220, G: 220, B: 220},
		FieldBg:  color.RGB{R: 35, G: 35, B: 45},
		FocusBg:  color.RGB{R: 45, G: 45, B: 60},
		CursorFg: color.RGB{R: 0, G: 0, B: 0},
		CursorBg: color.RGB{R: 200, G: 200, B: 200},
		Bg:       color.RGB{R: 25, G: 25, B: 35},
	}
}

// Form renders a multi-field form with labels and editable text fields, returns height used
func (r Region) Form(state *FormState, opts FormOpts) int {
	if len(state.Fields) == 0 || r.H < 1 {
		return 0
	}

	style := opts.Style
	if style == (FormStyle{}) {
		style = DefaultFormStyle()
	}

	labelW := opts.LabelWidth
	if labelW <= 0 {
		for _, f := range state.Fields {
			if w := RuneLen(f.Label); w > labelW {
				labelW = w
			}
		}
		labelW += 2
	}

	spacing := opts.Spacing
	if spacing < 1 {
		spacing = 1
	}

	y := 0
	for i, field := range state.Fields {
		if y >= r.H {
			break
		}

		isFocused := i == state.Focus

		// Label
		label := field.Label + ":"
		for j, ch := range label {
			if j >= labelW || j >= r.W {
				break
			}
			r.Cell(j, y, ch, style.LabelFg, style.Bg, terminal.AttrNone)
		}

		// Field area
		fieldX := labelW
		fieldW := r.W - labelW
		if fieldW < 1 {
			y += spacing
			continue
		}

		fieldBg := style.FieldBg
		if isFocused {
			fieldBg = style.FocusBg
		}

		for x := fieldX; x < fieldX+fieldW && x < r.W; x++ {
			r.Cell(x, y, ' ', style.FieldFg, fieldBg, terminal.AttrNone)
		}

		// Render text using existing TextField logic
		text := field.State.Text
		scroll := field.State.Scroll
		cursor := field.State.Cursor

		field.State.AdjustScroll(fieldW)
		scroll = field.State.Scroll

		for x := 0; x < fieldW; x++ {
			runeIdx := scroll + x
			ch := ' '
			if runeIdx < len(text) {
				ch = text[runeIdx]
			}

			fg := style.FieldFg
			bg := fieldBg

			if isFocused && runeIdx == cursor {
				fg = style.CursorFg
				bg = style.CursorBg
			}

			if fieldX+x < r.W {
				r.Cell(fieldX+x, y, ch, fg, bg, terminal.AttrNone)
			}
		}

		// Cursor at end of text
		if isFocused && cursor == len(text) {
			cursorX := fieldX + cursor - scroll
			if cursorX >= fieldX && cursorX < fieldX+fieldW && cursorX < r.W {
				r.Cell(cursorX, y, ' ', style.CursorFg, style.CursorBg, terminal.AttrNone)
			}
		}

		y += spacing
	}

	return y
}

