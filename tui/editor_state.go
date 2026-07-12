package tui

import (
	"github.com/lixenwraith/terminal"
)

// splitLines splits string into lines by newline character
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

// EditorState holds multi-line text editor state
type EditorState struct {
	Lines      []string
	CursorLine int
	CursorCol  int
	ScrollX    int
	ScrollY    int
	ViewportW  int // Updated during render
	ViewportH  int // Updated during render
}

// NewEditorState creates initialized editor state
func NewEditorState(initial string) *EditorState {
	lines := splitLines(initial)
	if len(lines) == 0 {
		lines = []string{""}
	}
	return &EditorState{
		Lines: lines,
	}
}

// --- Value access ---

// Value returns all lines joined with newlines
func (e *EditorState) Value() string {
	if len(e.Lines) == 0 {
		return ""
	}
	result := e.Lines[0]
	for i := 1; i < len(e.Lines); i++ {
		result += "\n" + e.Lines[i]
	}
	return result
}

// SetValue replaces all content and resets cursor
func (e *EditorState) SetValue(s string) {
	e.Lines = splitLines(s)
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	e.CursorLine = 0
	e.CursorCol = 0
	e.ScrollX = 0
	e.ScrollY = 0
}

// Clear empties the editor
func (e *EditorState) Clear() {
	e.Lines = []string{""}
	e.CursorLine = 0
	e.CursorCol = 0
	e.ScrollX = 0
	e.ScrollY = 0
}

// --- Line queries ---

// LineCount returns number of lines
func (e *EditorState) LineCount() int {
	return len(e.Lines)
}

// CurrentLine returns current line text
func (e *EditorState) CurrentLine() string {
	if e.CursorLine < 0 || e.CursorLine >= len(e.Lines) {
		return ""
	}
	return e.Lines[e.CursorLine]
}

// --- Cursor clamping ---

// clampCursor ensures cursor is within valid bounds
func (e *EditorState) clampCursor() {
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	if e.CursorLine < 0 {
		e.CursorLine = 0
	}
	if e.CursorLine >= len(e.Lines) {
		e.CursorLine = len(e.Lines) - 1
	}
	lineLen := len([]rune(e.Lines[e.CursorLine]))
	if e.CursorCol < 0 {
		e.CursorCol = 0
	}
	if e.CursorCol > lineLen {
		e.CursorCol = lineLen
	}
}

// --- Character insertion ---

// Insert adds a rune at cursor position
func (e *EditorState) Insert(r rune) {
	e.clampCursor()
	line := []rune(e.Lines[e.CursorLine])
	line = append(line[:e.CursorCol], append([]rune{r}, line[e.CursorCol:]...)...)
	e.Lines[e.CursorLine] = string(line)
	e.CursorCol++
}

// InsertString adds string at cursor, handling newlines
func (e *EditorState) InsertString(s string) {
	for _, r := range s {
		if r == '\n' {
			e.InsertNewline()
		} else {
			e.Insert(r)
		}
	}
}

// InsertNewline splits current line at cursor
func (e *EditorState) InsertNewline() {
	e.clampCursor()
	runes := []rune(e.Lines[e.CursorLine])

	before := string(runes[:e.CursorCol])
	after := string(runes[e.CursorCol:])

	e.Lines[e.CursorLine] = before
	e.Lines = append(e.Lines[:e.CursorLine+1], append([]string{after}, e.Lines[e.CursorLine+1:]...)...)
	e.CursorLine++
	e.CursorCol = 0
}

// --- Character deletion ---

// DeleteBackward deletes character before cursor or merges lines
func (e *EditorState) DeleteBackward() bool {
	e.clampCursor()
	if e.CursorCol > 0 {
		line := []rune(e.Lines[e.CursorLine])
		line = append(line[:e.CursorCol-1], line[e.CursorCol:]...)
		e.Lines[e.CursorLine] = string(line)
		e.CursorCol--
		return true
	}
	if e.CursorLine > 0 {
		prevLine := e.Lines[e.CursorLine-1]
		curLine := e.Lines[e.CursorLine]
		newCol := len([]rune(prevLine))
		e.Lines[e.CursorLine-1] = prevLine + curLine
		e.Lines = append(e.Lines[:e.CursorLine], e.Lines[e.CursorLine+1:]...)
		e.CursorLine--
		e.CursorCol = newCol
		return true
	}
	return false
}

// DeleteForward deletes character at cursor or merges with next line
func (e *EditorState) DeleteForward() bool {
	e.clampCursor()
	line := []rune(e.Lines[e.CursorLine])
	if e.CursorCol < len(line) {
		line = append(line[:e.CursorCol], line[e.CursorCol+1:]...)
		e.Lines[e.CursorLine] = string(line)
		return true
	}
	if e.CursorLine < len(e.Lines)-1 {
		e.Lines[e.CursorLine] = e.Lines[e.CursorLine] + e.Lines[e.CursorLine+1]
		e.Lines = append(e.Lines[:e.CursorLine+1], e.Lines[e.CursorLine+2:]...)
		return true
	}
	return false
}

// --- Word deletion ---

// DeleteWordBackward deletes word before cursor
func (e *EditorState) DeleteWordBackward() bool {
	e.clampCursor()
	if e.CursorCol == 0 {
		return e.DeleteBackward()
	}

	line := []rune(e.Lines[e.CursorLine])
	end := e.CursorCol

	// Skip trailing non-word chars
	for end > 0 && !isWordChar(line[end-1]) {
		end--
	}
	// Skip word chars
	start := end
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	if start == e.CursorCol {
		start = e.CursorCol - 1
	}

	line = append(line[:start], line[e.CursorCol:]...)
	e.Lines[e.CursorLine] = string(line)
	e.CursorCol = start
	return true
}

// DeleteWordForward deletes word after cursor
func (e *EditorState) DeleteWordForward() bool {
	e.clampCursor()
	line := []rune(e.Lines[e.CursorLine])
	if e.CursorCol >= len(line) {
		return e.DeleteForward()
	}

	start := e.CursorCol
	end := start

	// Skip word chars
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	// Skip trailing non-word chars
	for end < len(line) && !isWordChar(line[end]) {
		end++
	}
	if end == start {
		end = start + 1
	}

	line = append(line[:start], line[end:]...)
	e.Lines[e.CursorLine] = string(line)
	return true
}

// Line deletion

// DeleteToEndOfLine deletes from cursor to end of line
func (e *EditorState) DeleteToEndOfLine() bool {
	e.clampCursor()
	line := []rune(e.Lines[e.CursorLine])
	if e.CursorCol < len(line) {
		e.Lines[e.CursorLine] = string(line[:e.CursorCol])
		return true
	}
	// At end of line - merge with next
	return e.DeleteForward()
}

// DeleteToStartOfLine deletes from start to cursor
func (e *EditorState) DeleteToStartOfLine() bool {
	e.clampCursor()
	if e.CursorCol > 0 {
		line := []rune(e.Lines[e.CursorLine])
		e.Lines[e.CursorLine] = string(line[e.CursorCol:])
		e.CursorCol = 0
		return true
	}
	return false
}

// DeleteLine removes current line
func (e *EditorState) DeleteLine() bool {
	if len(e.Lines) == 1 {
		e.Lines[0] = ""
		e.CursorCol = 0
		return true
	}
	e.Lines = append(e.Lines[:e.CursorLine], e.Lines[e.CursorLine+1:]...)
	e.clampCursor()
	return true
}

// --- Line navigation ---

// MoveUp moves cursor to previous line
func (e *EditorState) MoveUp() {
	if e.CursorLine > 0 {
		e.CursorLine--
		e.clampCursor()
	}
}

// MoveDown moves cursor to next line
func (e *EditorState) MoveDown() {
	if e.CursorLine < len(e.Lines)-1 {
		e.CursorLine++
		e.clampCursor()
	}
}

// --- Character navigation ---

// MoveLeft moves cursor left, wrapping to previous line
func (e *EditorState) MoveLeft() {
	if e.CursorCol > 0 {
		e.CursorCol--
	} else if e.CursorLine > 0 {
		e.CursorLine--
		e.CursorCol = len([]rune(e.Lines[e.CursorLine]))
	}
}

// MoveRight moves cursor right, wrapping to next line
func (e *EditorState) MoveRight() {
	lineLen := len([]rune(e.Lines[e.CursorLine]))
	if e.CursorCol < lineLen {
		e.CursorCol++
	} else if e.CursorLine < len(e.Lines)-1 {
		e.CursorLine++
		e.CursorCol = 0
	}
}

// --- Word navigation ---

// MoveWordLeft moves cursor to previous word boundary
func (e *EditorState) MoveWordLeft() {
	if e.CursorCol == 0 {
		if e.CursorLine > 0 {
			e.CursorLine--
			e.CursorCol = len([]rune(e.Lines[e.CursorLine]))
		}
		return
	}

	line := []rune(e.Lines[e.CursorLine])
	for e.CursorCol > 0 && !isWordChar(line[e.CursorCol-1]) {
		e.CursorCol--
	}
	for e.CursorCol > 0 && isWordChar(line[e.CursorCol-1]) {
		e.CursorCol--
	}
}

// MoveWordRight moves cursor to next word boundary
func (e *EditorState) MoveWordRight() {
	line := []rune(e.Lines[e.CursorLine])
	lineLen := len(line)

	if e.CursorCol >= lineLen {
		if e.CursorLine < len(e.Lines)-1 {
			e.CursorLine++
			e.CursorCol = 0
		}
		return
	}

	for e.CursorCol < lineLen && isWordChar(line[e.CursorCol]) {
		e.CursorCol++
	}
	for e.CursorCol < lineLen && !isWordChar(line[e.CursorCol]) {
		e.CursorCol++
	}
}

// --- Positions navigation ---

// MoveToLineStart moves cursor to start of line
func (e *EditorState) MoveToLineStart() {
	e.CursorCol = 0
}

// MoveToLineEnd moves cursor to end of line
func (e *EditorState) MoveToLineEnd() {
	e.CursorCol = len([]rune(e.Lines[e.CursorLine]))
}

// MoveToStart moves cursor to start of document
func (e *EditorState) MoveToStart() {
	e.CursorLine = 0
	e.CursorCol = 0
}

// MoveToEnd moves cursor to end of document
func (e *EditorState) MoveToEnd() {
	e.CursorLine = len(e.Lines) - 1
	e.CursorCol = len([]rune(e.Lines[e.CursorLine]))
}

// --- Page navigation ---

// PageUp moves cursor up by half viewport
func (e *EditorState) PageUp() {
	delta := e.ViewportH / 2
	if delta < 1 {
		delta = 1
	}
	e.CursorLine -= delta
	if e.CursorLine < 0 {
		e.CursorLine = 0
	}
	e.clampCursor()
}

// PageDown moves cursor down by half viewport
func (e *EditorState) PageDown() {
	delta := e.ViewportH / 2
	if delta < 1 {
		delta = 1
	}
	e.CursorLine += delta
	if e.CursorLine >= len(e.Lines) {
		e.CursorLine = len(e.Lines) - 1
	}
	e.clampCursor()
}

// --- Scroll management ---

// AdjustScroll updates scroll to keep cursor visible
func (e *EditorState) AdjustScroll(viewportW, viewportH int) {
	e.ViewportW = viewportW
	e.ViewportH = viewportH

	// Vertical
	if e.CursorLine < e.ScrollY {
		e.ScrollY = e.CursorLine
	}
	if e.CursorLine >= e.ScrollY+viewportH {
		e.ScrollY = e.CursorLine - viewportH + 1
	}
	if e.ScrollY < 0 {
		e.ScrollY = 0
	}

	// Horizontal
	if e.CursorCol < e.ScrollX {
		e.ScrollX = e.CursorCol
	}
	if e.CursorCol >= e.ScrollX+viewportW {
		e.ScrollX = e.CursorCol - viewportW + 1
	}
	if e.ScrollX < 0 {
		e.ScrollX = 0
	}
}

// --- Input handling ---

// HandleKey processes keyboard input, returns true if state changed
func (e *EditorState) HandleKey(key terminal.Key, r rune, mod terminal.Modifier) bool {
	switch key {
	case terminal.KeyUp:
		e.MoveUp()
		return true
	case terminal.KeyDown:
		e.MoveDown()
		return true
	case terminal.KeyLeft:
		if mod&terminal.ModCtrl != 0 {
			e.MoveWordLeft()
		} else {
			e.MoveLeft()
		}
		return true
	case terminal.KeyRight:
		if mod&terminal.ModCtrl != 0 {
			e.MoveWordRight()
		} else {
			e.MoveRight()
		}
		return true
	case terminal.KeyHome:
		if mod&terminal.ModCtrl != 0 {
			e.MoveToStart()
		} else {
			e.MoveToLineStart()
		}
		return true
	case terminal.KeyEnd:
		if mod&terminal.ModCtrl != 0 {
			e.MoveToEnd()
		} else {
			e.MoveToLineEnd()
		}
		return true
	case terminal.KeyPageUp:
		e.PageUp()
		return true
	case terminal.KeyPageDown:
		e.PageDown()
		return true
	case terminal.KeyEnter:
		e.InsertNewline()
		return true
	case terminal.KeyBackspace:
		if mod&terminal.ModCtrl != 0 {
			return e.DeleteWordBackward()
		}
		return e.DeleteBackward()
	case terminal.KeyDelete:
		if mod&terminal.ModCtrl != 0 {
			return e.DeleteWordForward()
		}
		return e.DeleteForward()
	case terminal.KeyCtrlA:
		e.MoveToLineStart()
		return true
	case terminal.KeyCtrlE:
		e.MoveToLineEnd()
		return true
	case terminal.KeyCtrlK:
		return e.DeleteToEndOfLine()
	case terminal.KeyCtrlU:
		return e.DeleteToStartOfLine()
	case terminal.KeyCtrlW:
		return e.DeleteWordBackward()
	case terminal.KeyRune:
		if r >= 32 {
			e.Insert(r)
			return true
		}
	}
	return false
}