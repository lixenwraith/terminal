package tui

import (
	"strings"

	"github.com/lixenwraith/terminal"
)

// Option is one entry of an option list
type Option struct {
	Name string // what is chosen
	Hint string // a line about it
}

// OptionListState filters options by the text typed into it, matching name
// or hint regardless of case, and keeps a cursor on the matches
type OptionListState struct {
	Options []Option
	Filter  TextFieldState
	Cursor  int // index into Matches
	Scroll  int // first match shown
}

// NewOptionListState creates a list showing every option
func NewOptionListState(options []Option) *OptionListState {
	return &OptionListState{Options: options}
}

// Matches returns the indices of the options the filter matches, in order,
// keeping the cursor on one of them
func (s *OptionListState) Matches() []int {
	filter := strings.ToLower(s.Filter.Value())
	var m []int
	for i, o := range s.Options {
		if strings.Contains(strings.ToLower(o.Name), filter) || strings.Contains(strings.ToLower(o.Hint), filter) {
			m = append(m, i)
		}
	}
	s.Cursor = max(0, min(s.Cursor, len(m)-1))
	return m
}

// Chosen returns the index in Options of the option under the cursor, and
// false when nothing matches
func (s *OptionListState) Chosen() (int, bool) {
	m := s.Matches()
	if len(m) == 0 {
		return 0, false
	}
	return m[s.Cursor], true
}

// HandleKey moves the cursor on Up and Down, or edits the filter, and
// reports whether anything changed
func (s *OptionListState) HandleKey(key terminal.Key, r rune, mod terminal.Modifier) bool {
	filter := s.Filter.Value()
	switch {
	case key == terminal.KeyUp:
		s.Cursor--
	case key == terminal.KeyDown:
		s.Cursor++
	case !s.Filter.HandleKey(key, r, mod):
		return false
	case s.Filter.Value() != filter:
		s.Cursor = 0
	}
	s.Matches() // keeps the cursor on a match
	return true
}

// Paste adds pasted text to the filter
func (s *OptionListState) Paste(text string) bool {
	if !s.Filter.Paste(text) {
		return false
	}
	s.Cursor = 0
	return true
}

// OptionList draws the filter on the first row and the matches below it,
// the cursor row marked with the pointer on the Selected background, each
// hint muted in a column after the names
func (r Region) OptionList(s *OptionListState, th Theme) {
	if r.W < 4 || r.H < 2 {
		return
	}
	r.TextStyled(0, 0, "/", th.Muted)
	r.Sub(2, 0, r.W-2, 1).TextInput(&s.Filter, "type to filter", true, th)

	m := s.Matches()
	rows := r.H - 1
	if len(m) == 0 {
		r.TextStyled(2, 1, "no match", th.Muted)
		return
	}
	s.Scroll = max(0, min(s.Scroll, s.Cursor, len(m)-rows))
	s.Scroll = max(s.Scroll, s.Cursor-rows+1)

	nameW := 0
	for _, i := range m {
		nameW = max(nameW, RuneLen(s.Options[i].Name))
	}
	nameW = min(nameW, r.W/2)
	for y := 0; y < rows && s.Scroll+y < len(m); y++ {
		o := s.Options[m[s.Scroll+y]]
		row := r.Sub(0, 1+y, r.W, 1)
		var bg Style
		if s.Scroll+y == s.Cursor {
			bg = th.Selected
			row.FillStyle(th.Text.On(bg))
			row.TextStyled(0, 0, string(th.Glyphs.Pointer), th.Accent.On(bg))
		}
		row.TextStyled(2, 0, Truncate(o.Name, nameW), th.Text.On(bg))
		row.TextStyled(2+nameW+2, 0, Truncate(o.Hint, r.W-nameW-4), th.Muted.On(bg))
	}
}
