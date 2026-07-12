package tui

import (
	"unicode"

	"github.com/lixenwraith/terminal"
)

// isWordChar returns true for word-constituent characters
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// TextFieldState holds editable text field state
type TextFieldState struct {
	Text   []rune
	Cursor int // Positions before which cursor sits (0 = before first char)
	Scroll int // First visible rune index
}

// NewTextFieldState creates initialized text field state
func NewTextFieldState(initial string) *TextFieldState {
	runes := []rune(initial)
	return &TextFieldState{
		Text:   runes,
		Cursor: len(runes),
		Scroll: 0,
	}
}

// --- Value access ---

// Value returns current text as string
func (t *TextFieldState) Value() string {
	return string(t.Text)
}

// SetValue replaces text and moves cursor to end
func (t *TextFieldState) SetValue(s string) {
	t.Text = []rune(s)
	t.Cursor = len(t.Text)
	t.Scroll = 0
}

// Clear empties the field
func (t *TextFieldState) Clear() {
	t.Text = nil
	t.Cursor = 0
	t.Scroll = 0
}

// --- Character insertion ---

// Insert adds rune at cursor position
func (t *TextFieldState) Insert(r rune) {
	t.Text = append(t.Text[:t.Cursor], append([]rune{r}, t.Text[t.Cursor:]...)...)
	t.Cursor++
}

// InsertString adds string at cursor position
func (t *TextFieldState) InsertString(s string) {
	runes := []rune(s)
	t.Text = append(t.Text[:t.Cursor], append(runes, t.Text[t.Cursor:]...)...)
	t.Cursor += len(runes)
}

// --- Character deletion ---

// DeleteBackward removes rune before cursor
func (t *TextFieldState) DeleteBackward() bool {
	if t.Cursor > 0 {
		t.Text = append(t.Text[:t.Cursor-1], t.Text[t.Cursor:]...)
		t.Cursor--
		return true
	}
	return false
}

// DeleteForward removes rune at cursor
func (t *TextFieldState) DeleteForward() bool {
	if t.Cursor < len(t.Text) {
		t.Text = append(t.Text[:t.Cursor], t.Text[t.Cursor+1:]...)
		return true
	}
	return false
}

// --- Word deletion ---

// DeleteWordBackward removes word before cursor
func (t *TextFieldState) DeleteWordBackward() bool {
	if t.Cursor == 0 {
		return false
	}
	// Skip trailing non-word chars
	end := t.Cursor
	for end > 0 && !isWordChar(t.Text[end-1]) {
		end--
	}
	// Skip word chars
	start := end
	for start > 0 && isWordChar(t.Text[start-1]) {
		start--
	}
	if start == t.Cursor {
		start = t.Cursor - 1
	}
	t.Text = append(t.Text[:start], t.Text[t.Cursor:]...)
	t.Cursor = start
	return true
}

// DeleteWordForward removes word after cursor
func (t *TextFieldState) DeleteWordForward() bool {
	if t.Cursor >= len(t.Text) {
		return false
	}
	// Skip word chars
	end := t.Cursor
	for end < len(t.Text) && isWordChar(t.Text[end]) {
		end++
	}
	// Skip trailing non-word chars
	for end < len(t.Text) && !isWordChar(t.Text[end]) {
		end++
	}
	if end == t.Cursor {
		end = t.Cursor + 1
	}
	t.Text = append(t.Text[:t.Cursor], t.Text[end:]...)
	return true
}

// DeleteToEnd removes from cursor to end
func (t *TextFieldState) DeleteToEnd() bool {
	if t.Cursor < len(t.Text) {
		t.Text = t.Text[:t.Cursor]
		return true
	}
	return false
}

// DeleteToStart removes from start to cursor
func (t *TextFieldState) DeleteToStart() bool {
	if t.Cursor > 0 {
		t.Text = t.Text[t.Cursor:]
		t.Cursor = 0
		t.Scroll = 0
		return true
	}
	return false
}

// --- Character movement ---

// MoveLeft moves cursor left
func (t *TextFieldState) MoveLeft() {
	if t.Cursor > 0 {
		t.Cursor--
	}
}

// MoveRight moves cursor right
func (t *TextFieldState) MoveRight() {
	if t.Cursor < len(t.Text) {
		t.Cursor++
	}
}

// --- Word movement ---

// MoveWordLeft moves cursor to previous word boundary
func (t *TextFieldState) MoveWordLeft() {
	if t.Cursor == 0 {
		return
	}
	// Skip non-word chars
	for t.Cursor > 0 && !isWordChar(t.Text[t.Cursor-1]) {
		t.Cursor--
	}
	// Skip word chars
	for t.Cursor > 0 && isWordChar(t.Text[t.Cursor-1]) {
		t.Cursor--
	}
}

// MoveWordRight moves cursor to next word boundary
func (t *TextFieldState) MoveWordRight() {
	if t.Cursor >= len(t.Text) {
		return
	}
	// Skip word chars
	for t.Cursor < len(t.Text) && isWordChar(t.Text[t.Cursor]) {
		t.Cursor++
	}
	// Skip non-word chars
	for t.Cursor < len(t.Text) && !isWordChar(t.Text[t.Cursor]) {
		t.Cursor++
	}
}

// --- Line movement ---

// MoveToStart moves cursor to beginning
func (t *TextFieldState) MoveToStart() {
	t.Cursor = 0
}

// MoveToEnd moves cursor to end
func (t *TextFieldState) MoveToEnd() {
	t.Cursor = len(t.Text)
}

// --- Scroll management ---

// AdjustScroll updates scroll to keep cursor visible within viewport width
func (t *TextFieldState) AdjustScroll(viewportW int) {
	if viewportW <= 0 {
		return
	}
	if t.Cursor < t.Scroll {
		t.Scroll = t.Cursor
	}
	if t.Cursor >= t.Scroll+viewportW {
		t.Scroll = t.Cursor - viewportW + 1
	}
	if t.Scroll < 0 {
		t.Scroll = 0
	}
}

// --- Input handling ---

// HandleKey processes keyboard input, returns true if state changed
func (t *TextFieldState) HandleKey(key terminal.Key, r rune, mod terminal.Modifier) bool {
	switch key {
	case terminal.KeyLeft:
		if mod&terminal.ModCtrl != 0 {
			t.MoveWordLeft()
		} else {
			t.MoveLeft()
		}
		return true
	case terminal.KeyRight:
		if mod&terminal.ModCtrl != 0 {
			t.MoveWordRight()
		} else {
			t.MoveRight()
		}
		return true
	case terminal.KeyHome, terminal.KeyCtrlA:
		t.MoveToStart()
		return true
	case terminal.KeyEnd, terminal.KeyCtrlE:
		t.MoveToEnd()
		return true
	case terminal.KeyBackspace:
		if mod&terminal.ModCtrl != 0 {
			return t.DeleteWordBackward()
		}
		return t.DeleteBackward()
	case terminal.KeyDelete:
		if mod&terminal.ModCtrl != 0 {
			return t.DeleteWordForward()
		}
		return t.DeleteForward()
	case terminal.KeyCtrlK:
		return t.DeleteToEnd()
	case terminal.KeyCtrlU:
		return t.DeleteToStart()
	case terminal.KeyCtrlW:
		return t.DeleteWordBackward()
	case terminal.KeyRune:
		if r >= 32 { // Printable
			t.Insert(r)
			return true
		}
	}
	return false
}