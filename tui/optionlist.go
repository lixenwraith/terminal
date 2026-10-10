package tui

import (
	"strings"

	"github.com/lixenwraith/terminal"
)

// Option is one entry of an option list
type Option struct {
	Name string // what is chosen
	Hint string // a line about it
	Key  rune   // in a menu, the key that picks it at once
}

// OptionListState filters options by the text typed into it, matching name
// or hint regardless of case, and keeps a cursor on the matches. A menu has
// no filter: each option's Key picks it.
type OptionListState struct {
	Options []Option
	Filter  TextFieldState
	Cursor  int  // index into Matches
	Scroll  int  // first match shown
	Menu    bool // no filter; Pick finds an option by its Key
}

// minHintW is the narrowest hint column beside the names; narrower, the
// cursor's hint wraps below the list instead
const minHintW = 16

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
	case s.Menu || !s.Filter.HandleKey(key, r, mod):
		return false
	case s.Filter.Value() != filter:
		s.Cursor = 0
	}
	s.Matches() // keeps the cursor on a match
	return true
}

// Pick returns the index in Options of the option whose Key is r
func (s *OptionListState) Pick(r rune) (int, bool) {
	for i, o := range s.Options {
		if o.Key == r && r != 0 {
			return i, true
		}
	}
	return 0, false
}

// Paste adds pasted text to the filter
func (s *OptionListState) Paste(text string) bool {
	if s.Menu || !s.Filter.Paste(text) {
		return false
	}
	s.Cursor = 0
	return true
}

// layout is how the list fits w columns: the names' width, the column the
// names start at, and the rows below the list that hold the cursor's hint
// where hints do not fit beside the names
func (s *OptionListState) layout(w int) (nameW, nameX, hintRows int) {
	nameX = 2
	if s.Menu {
		nameX = 4
	}
	for _, o := range s.Options {
		nameW = max(nameW, RuneLen(o.Name))
	}
	nameW = min(nameW, (w-nameX)/2)
	if w-nameX-nameW-2 >= minHintW {
		return nameW, nameX, 0
	}
	nameW = w - nameX
	for _, o := range s.Options {
		if o.Hint != "" {
			hintRows = max(hintRows, min(3, len(WrapText(o.Hint, max(1, w-2)))))
		}
	}
	return nameW, nameX, hintRows
}

// Rows returns the rows the list takes at width w to show every option: the
// filter, a row each, and the cursor's hint where it does not fit beside them
func (s *OptionListState) Rows(w int) int {
	_, _, hintRows := s.layout(w)
	return 1 - boolToInt(s.Menu) + len(s.Options) + hintRows
}

// OptionList draws the filter on the first row, a menu none, and the
// matches below, the cursor row marked with the pointer on the Selected
// background and a menu's keys before the names in the accent style. Hints
// sit muted beside the names or, where that column would be narrower than
// 16, the cursor's hint wraps onto the list's last rows.
func (r Region) OptionList(s *OptionListState, th Theme) {
	if r.W < 4 || r.H < 2 {
		return
	}
	top := 1
	if s.Menu {
		top = 0
	} else {
		r.TextStyled(0, 0, "/", th.Muted)
		r.Sub(2, 0, r.W-2, 1).TextInput(&s.Filter, "type to filter", true, th)
	}
	m := s.Matches()
	if len(m) == 0 {
		r.TextStyled(2, top, "no match", th.Muted)
		return
	}
	nameW, nameX, hintRows := s.layout(r.W)
	rows := max(1, r.H-top-hintRows)
	s.Scroll = AdjustScroll(s.Cursor, ClampScroll(s.Scroll, rows, len(m)), rows, len(m))
	for y := 0; y < rows && s.Scroll+y < len(m); y++ {
		o := s.Options[m[s.Scroll+y]]
		row := r.Sub(0, top+y, r.W, 1)
		var bg Style
		if s.Scroll+y == s.Cursor {
			bg = th.Selected
			row.FillStyle(th.Text.On(bg))
			row.TextStyled(0, 0, string(th.Glyphs.Pointer), th.Accent.On(bg))
		}
		if s.Menu && o.Key != 0 {
			row.TextStyled(2, 0, string(o.Key), th.Accent.On(bg))
		}
		row.TextStyled(nameX, 0, Truncate(o.Name, nameW), th.Text.On(bg))
		if hintRows == 0 {
			row.TextStyled(nameX+nameW+2, 0, Truncate(o.Hint, r.W-nameX-nameW-2), th.Muted.On(bg))
		}
	}
	hint, n := WrapText(s.Options[m[s.Cursor]].Hint, max(1, r.W-2)), min(hintRows, r.H-top-rows)
	if len(hint) > n && n > 0 { // the last row shown ends in '…'
		hint = append(hint[:n-1], Truncate(hint[n-1]+" "+hint[n], r.W-2))
	}
	for y, l := range hint[:min(n, len(hint))] { // on the last rows, so the list ends on text
		r.TextStyled(2, r.H-min(n, len(hint))+y, l, th.Muted)
	}
}
