package tui

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// canvas returns a w x h region filled with th's panel
func canvas(w, h int, th Theme) Region {
	r := NewRegion(make([]terminal.Cell, w*h), w, 0, 0, w, h)
	r.FillStyle(th.Text)
	return r
}

// row returns the runes of row y, trailing spaces trimmed
func row(r Region, y int) string {
	var b strings.Builder
	for x := 0; x < r.W; x++ {
		b.WriteRune(r.Cells[y*r.TotalW+x].Rune)
	}
	return strings.TrimRight(b.String(), " ")
}

// A transparent background keeps the kind of the one beneath it: text drawn,
// or a style put, on the terminal's own background or a palette color stays
// on it
func TestTransparentBackgroundKeepsItsKind(t *testing.T) {
	for _, panel := range []Style{MonoTheme.Text, {Bg: color.RGB{R: 4}, Attr: terminal.AttrBg256}} {
		r := canvas(4, 1, Theme{Text: panel})
		r.Text(0, 0, "ab", color.RGB{R: 1}, color.RGB{}, terminal.AttrBold)
		if c := r.Cells[0]; c.Bg != panel.Bg || c.Attrs != terminal.AttrBold|panel.Attr&terminal.AttrBgColor {
			t.Errorf("on %+v: %+v", panel, c)
		}
		if on := panel.On(Style{Attr: terminal.AttrReverse}); on.Bg != panel.Bg || on.Attr != panel.Attr|terminal.AttrReverse {
			t.Errorf("%+v on reverse: %+v", panel, on)
		}
	}
}

// Every widget marks its selection with a glyph, so it reads with no color
func TestSelectionChangesAGlyph(t *testing.T) {
	for _, th := range []Theme{DefaultTheme, Theme16, MonoTheme} {
		selectionChangesAGlyph(t, th)
	}
}

func selectionChangesAGlyph(t *testing.T, th Theme) {
	options := NewOptionListState([]Option{{Name: "file"}, {Name: "tcp"}})
	for name, draw := range map[string]func(r Region, on bool){
		"radio":  func(r Region, on bool) { r.Radio(0, 0, []string{"raw", "txt"}, map[bool]int{true: 1}[on], th) },
		"toggle": func(r Region, on bool) { r.Toggle(0, 0, on, th) },
		"field":  func(r Region, on bool) { r.Field(0, 6, Field{Label: "port"}, on, th) },
		"group":  func(r Region, on bool) { r.Group(0, "tls", "", on, false, th) },
		"option list": func(r Region, on bool) {
			options.Cursor = map[bool]int{true: 1}[on]
			r.OptionList(options, th)
		},
	} {
		off, on := canvas(20, 3, th), canvas(20, 3, th)
		draw(off, false)
		draw(on, true)
		if row(off, 0)+row(off, 1)+row(off, 2) == row(on, 0)+row(on, 1)+row(on, 2) {
			t.Errorf("%s: %q either way", name, row(on, 0)+"|"+row(on, 1))
		}
	}
}

// The 16-color theme names only ANSI colors and the terminal's own, which a
// text console shows as it shows everything else; ThemeFor picks it
func TestTheme16NamesOnlyANSIColors(t *testing.T) {
	ansiOrOwn := func(c color.RGB, attr, palette, own terminal.Attr, transparent bool) bool {
		return attr&own != 0 || attr&palette != 0 && c.R < 16 || transparent && c == (color.RGB{}) && attr&(palette|own) == 0
	}
	for _, s := range []Style{Theme16.Text, Theme16.Muted, Theme16.Accent, Theme16.Selected, Theme16.Input, Theme16.Cursor, Theme16.Error, Theme16.Border} {
		if !ansiOrOwn(s.Fg, s.Attr, terminal.AttrFg256, terminal.AttrFgDefault, false) || !ansiOrOwn(s.Bg, s.Attr, terminal.AttrBg256, terminal.AttrBgDefault, true) {
			t.Errorf("%+v", s)
		}
	}
	if ThemeFor(terminal.ColorMode16).Muted != Theme16.Muted || ThemeFor(terminal.ColorModeNone).Muted != MonoTheme.Muted ||
		ThemeFor(terminal.ColorModeTrueColor).Muted != DefaultTheme.Muted {
		t.Error("ThemeFor")
	}
}

// The option list matches name or hint regardless of case, a changed filter
// puts the cursor on the first match, and the cursor stays on the matches
func TestOptionListFilters(t *testing.T) {
	s := NewOptionListState([]Option{{Name: "file", Hint: "tail files"}, {Name: "Console", Hint: "stdin or stdout"},
		{Name: "tcp", Hint: "listen on TCP"}, {Name: "http", Hint: "Serve HTTP"}})
	for range 5 {
		s.HandleKey(terminal.KeyDown, 0, 0)
	}
	if i, _ := s.Chosen(); i != 3 {
		t.Fatalf("past the end: %d", i)
	}
	s.Paste("t")
	if m := s.Matches(); len(m) != 4 || s.Cursor != 0 {
		t.Fatalf("t: %v cursor %d", m, s.Cursor)
	}
	s.HandleKey(terminal.KeyDown, 0, 0)
	if s.HandleKey(terminal.KeyLeft, 0, 0); s.Cursor != 1 {
		t.Fatalf("filter cursor moved: list cursor %d", s.Cursor)
	}
	if s.HandleKey(terminal.KeyBackspace, 0, 0); s.Filter.Value() != "t" || s.Cursor != 1 {
		t.Fatalf("nothing before the filter cursor: %q cursor %d", s.Filter.Value(), s.Cursor)
	}
	s.HandleKey(terminal.KeyRight, 0, 0)
	if s.HandleKey(terminal.KeyBackspace, 0, 0); s.Filter.Value() != "" || s.Cursor != 0 {
		t.Fatalf("erased: %q cursor %d", s.Filter.Value(), s.Cursor)
	}
	for filter, want := range map[string]int{"TCP": 2, "cons": 1, "serve h": 3} {
		s.Filter.SetValue(filter)
		if m := s.Matches(); len(m) != 1 || m[0] != want {
			t.Errorf("%s: %v", filter, m)
		}
	}
	s.Filter.SetValue("udp")
	if _, ok := s.Chosen(); ok {
		t.Fatal("udp matched")
	}
}

// A field holds no control character, which the terminal or a file it is
// saved to would act on: pasted line breaks and tabs become spaces, other
// controls, pasted or typed, are dropped
func TestFieldHoldsNoControls(t *testing.T) {
	var f TextFieldState
	f.Paste("a\x1b[31mb\r\nc\td\u009b")
	f.HandleKey(terminal.KeyRune, 0x9b, 0)
	f.HandleKey(terminal.KeyRune, 0x85, 0)
	if got := f.Value(); got != "a[31mb c d" {
		t.Fatalf("%q", got)
	}
}

// A field whose text the caller replaced draws and takes input without
// panicking: the cursor stays within the text
func TestTextInputSurvivesReplacedText(t *testing.T) {
	f := NewTextFieldState("abcdefghij")
	canvas(4, 1, DefaultTheme).TextInput(f, "", true, DefaultTheme)
	f.Text = []rune("ab")
	canvas(4, 1, DefaultTheme).TextInput(f, "", true, DefaultTheme)
	f.Cursor = 9
	f.Paste("x")
	type press struct {
		key terminal.Key
		mod terminal.Modifier
	}
	ctrl := terminal.ModCtrl
	for cursor, keys := range map[int][]press{
		9:  {{terminal.KeyBackspace, 0}, {terminal.KeyCtrlW, 0}, {terminal.KeyCtrlU, 0}, {terminal.KeyLeft, ctrl}, {terminal.KeyRight, 0}},
		-1: {{terminal.KeyDelete, 0}, {terminal.KeyDelete, ctrl}, {terminal.KeyRight, ctrl}, {terminal.KeyLeft, 0}, {terminal.KeyCtrlK, 0}},
	} {
		for _, k := range keys {
			f.Text, f.Cursor = []rune("a b"), cursor
			f.HandleKey(k.key, 0, k.mod)
		}
	}
	f.Text, f.Cursor = []rune("a b"), -1
	f.HandleKey(terminal.KeyCtrlK, 0, 0)
	if got := f.Value(); got != "" {
		t.Fatalf("%q", got)
	}
}

// A typed field takes only its runes, typed or pasted
func TestTypedFieldTakesItsRunes(t *testing.T) {
	f := TextFieldState{Accept: AcceptInteger}
	for _, r := range "4a2" {
		f.HandleKey(terminal.KeyRune, r, 0)
	}
	f.Paste("1 0x8")
	if got := f.Value(); got != "42108" {
		t.Fatalf("%q", got)
	}
}

// Tab and Shift+Tab move focus around the ring, wrapping either way
func TestFocusRingWraps(t *testing.T) {
	f := Focus{Len: 3}
	f.HandleKey(terminal.KeyBacktab, terminal.ModShift)
	back := f.Index
	f.HandleKey(terminal.KeyTab, terminal.ModShift)
	shifted := f.Index
	for range 2 {
		f.HandleKey(terminal.KeyTab, 0)
	}
	if back != 2 || shifted != 1 || f.Index != 0 || f.HandleKey(terminal.KeyEnter, 0) {
		t.Fatalf("back %d, shifted %d, then %d", back, shifted, f.Index)
	}
}

// A field shows its error whether or not it has focus, its help only with
// focus, and its value beside the label column
func TestFieldShowsErrorOrHelp(t *testing.T) {
	f := Field{Label: "port", Required: true, Help: "listen port"}
	r := canvas(30, 2, DefaultTheme)
	value, rows := r.Field(0, 6, f, false, DefaultTheme)
	if rows != 1 || value.X != 9 || row(r, 0) != "  port *" {
		t.Fatalf("unfocused: %d rows, value at %d, %q", rows, value.X, row(r, 0))
	}
	r = canvas(30, 2, DefaultTheme)
	if _, rows = r.Field(0, 6, f, true, DefaultTheme); rows != 2 || row(r, 1) != "         listen port" {
		t.Fatalf("focused: %d rows, %q", rows, row(r, 1))
	}
	f.Error = "port: want 1-65535"
	r = canvas(30, 2, DefaultTheme)
	if _, rows = r.Field(0, 6, f, false, DefaultTheme); rows != 2 || !strings.HasSuffix(row(r, 1), "want 1-65535") {
		t.Fatalf("error: %d rows, %q", rows, row(r, 1))
	}
}

// An empty input shows its default, muted, with the cursor over it; typed
// text replaces it
func TestTextInputShowsTheDefaultUntilTyped(t *testing.T) {
	var f TextFieldState
	r := canvas(10, 1, DefaultTheme)
	r.TextInput(&f, "8080", true, DefaultTheme)
	if c := r.Cells[1]; row(r, 0) != "8080" || c.Fg != DefaultTheme.Muted.Fg || r.Cells[0].Bg != DefaultTheme.Cursor.Bg {
		t.Fatalf("empty: %q %+v", row(r, 0), c)
	}
	f.HandleKey(terminal.KeyRune, '9', 0)
	r.TextInput(&f, "8080", true, DefaultTheme)
	if c := r.Cells[0]; row(r, 0) != "9" || c.Fg != DefaultTheme.Text.Fg || c.Bg != DefaultTheme.Input.Bg {
		t.Fatalf("typed: %q %+v", row(r, 0), c)
	}
}

// A choice moves with Left and Right, or h and l, and stops at either end
func TestChoiceStopsAtTheEnds(t *testing.T) {
	i, _ := StepChoice(terminal.KeyRune, 'l', 0, 2)
	j, moved := StepChoice(terminal.KeyRight, 0, i, 2)
	k, _ := StepChoice(terminal.KeyLeft, 0, j, 2)
	if i != 1 || j != 1 || moved || k != 0 {
		t.Fatalf("%d %d %v %d", i, j, moved, k)
	}
}

// A folded group says what it holds; an open one shows it instead
func TestGroupSummarisesWhileFolded(t *testing.T) {
	folded, open := canvas(30, 1, MonoTheme), canvas(30, 1, MonoTheme)
	folded.Group(0, "tls", "on, pinned", false, false, MonoTheme)
	open.Group(0, "tls", "on, pinned", true, false, MonoTheme)
	if row(folded, 0) != "  ▸ tls  on, pinned" || row(open, 0) != "  ▾ tls" {
		t.Fatalf("%q %q", row(folded, 0), row(open, 0))
	}
}

// A radio row too wide for its region shows the chosen option and those
// around it, a '…' on each side where others are hidden, and reports the
// columns drawn
func TestRadioKeepsTheChosenInView(t *testing.T) {
	options := []string{"alpha", "beta", "gamma", "delta"}
	for _, c := range []struct {
		chosen, w int
		want      string
	}{
		{3, 40, "○ alpha  ○ beta  ○ gamma  ● delta"},
		{3, 30, "… ○ beta  ○ gamma  ● delta"},
		{3, 16, "… ● delta"},
		{3, 7, "… ● de…"},
		{0, 17, "● alpha  …"},
		{1, 19, "○ alpha  ● beta  …"},
		{1, 25, "… ● beta  ○ gamma  …"},
	} {
		r := canvas(c.w, 1, MonoTheme)
		if n := r.Radio(0, 0, options, c.chosen, MonoTheme); row(r, 0) != c.want || n != min(c.w, RuneLen(c.want)) {
			t.Errorf("%d chosen in %d columns: %q, %d drawn; want %q", c.chosen, c.w, row(r, 0), n, c.want)
		}
	}
}

// A field's view pulls back when its text shrinks, so no column is left
// blank while text is hidden, and a muted '…' marks text out of view, never
// under the cursor
func TestTextInputPullsBackAndMarksHiddenText(t *testing.T) {
	f := NewTextFieldState(strings.Repeat("abcdefghij", 2))
	draw := func(focused bool) string {
		r := canvas(10, 1, MonoTheme)
		r.TextInput(f, "", focused, MonoTheme)
		return row(r, 0)
	}
	if got := draw(true); got != "…cdefghij" {
		t.Fatalf("the cursor after 20 in 10 columns: %q", got)
	}
	for range 8 {
		f.HandleKey(terminal.KeyBackspace, 0, 0)
	}
	if got := draw(true); got != "…efghijab" {
		t.Fatalf("12 left: %q, scroll %d", got, f.Scroll)
	}
	f.HandleKey(terminal.KeyHome, 0, 0)
	if got := draw(true); got != "abcdefghi…" {
		t.Fatalf("the cursor at the start: %q", got)
	}
}

// A field's label leaves the value at least half the row; label width 0
// stacks the label on its own row, whole, and the value under it
func TestFieldLeavesTheValueHalfTheRow(t *testing.T) {
	for _, w := range []int{12, 30, 80} {
		r := canvas(w, 3, MonoTheme)
		if v, rows := r.Field(0, 40, Field{Label: "password_file"}, false, MonoTheme); v.W < (w-3)/2 || rows != 1 {
			t.Errorf("%d columns: value %d wide, %d rows", w, v.W, rows)
		}
		r = canvas(w, 3, MonoTheme)
		v, rows := r.Field(0, 0, Field{Label: "password_file"}, false, MonoTheme)
		if label := Truncate("password_file", w-2); row(r, 0) != "  "+label || v.Y != 1 || v.W != w-4 || rows != 2 {
			t.Errorf("%d columns stacked: %q, value at row %d, %d wide, %d rows", w, row(r, 0), v.Y, v.W, rows)
		}
	}
}

// Text wraps at each line break, keeping an empty line, and a word longer
// than the line breaks after a '.', ',', ':', '/', '[' or ']'
func TestWrapTextBreaksAtLinesAndMarks(t *testing.T) {
	for text, want := range map[string][]string{
		"ab\ncd ef\n\ngh":                      {"ab", "cd ef", "", "gh"},
		"key: /etc/logwisp/very/long/path.pem": {"key:", "/etc/logwisp/very/", "long/path.pem"},
	} {
		if got := WrapText(text, map[bool]int{true: 5, false: 20}[strings.HasPrefix(text, "ab")]); !slices.Equal(got, want) {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
}

// Wrapping is linear in the lines: 4000 lines allocate under 1 MiB
func TestWrapTextIsLinearInLines(t *testing.T) {
	text := strings.Repeat("x\n", 4000)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	lines := WrapText(text, 80)
	runtime.ReadMemStats(&after)
	if len(lines) != 4001 || after.TotalAlloc-before.TotalAlloc > 1<<20 {
		t.Fatalf("%d lines, %d bytes allocated", len(lines), after.TotalAlloc-before.TotalAlloc)
	}
}

// Where the hint column would be narrow, the list shows names alone and the
// cursor's hint wraps below them, in rows Rows counts; wide, hints sit beside
func TestNarrowOptionListShowsTheCursorsHint(t *testing.T) {
	s := NewOptionListState([]Option{{Name: "pipe", Hint: "standard input to standard output"}, {Name: "tail", Hint: "follow files"}})
	s.Cursor = 1
	for w, want := range map[int][]string{
		60: {"/ type to filter", "  pipe  standard input to standard output", "▸ tail  follow files"},
		20: {"/ type to filter", "  pipe", "▸ tail", "", "  follow files"},
	} {
		r := canvas(w, s.Rows(w), MonoTheme)
		r.OptionList(s, MonoTheme)
		var got []string
		for y := range r.H {
			got = append(got, row(r, y))
		}
		if !slices.Equal(got, want) {
			t.Errorf("%d columns: %q, want %q", w, got, want)
		}
	}
}

// A menu has no filter: a key picks its option, and typing filters nothing
func TestMenuPicksByKey(t *testing.T) {
	s := NewOptionListState([]Option{{Name: "run", Key: 'r'}, {Name: "command", Key: 'c'}})
	s.Menu = true
	if i, ok := s.Pick('c'); !ok || i != 1 {
		t.Fatalf("c: %d, %v", i, ok)
	}
	if _, ok := s.Pick('x'); ok || s.HandleKey(terminal.KeyRune, 'x', 0) || s.Paste("r") || s.Filter.Value() != "" {
		t.Fatalf("x picked or filtered: %q", s.Filter.Value())
	}
	r := canvas(20, s.Rows(20), MonoTheme)
	r.OptionList(s, MonoTheme)
	if r.H != 2 || row(r, 0) != "▸ r run" || row(r, 1) != "  c command" {
		t.Fatalf("%d rows: %q %q", r.H, row(r, 0), row(r, 1))
	}
}

// A frame's inside is the region within its border and padding, so a frame
// sized to what it holds ends on it; the border takes the glyphs' line type
func TestFrameHoldsItsInside(t *testing.T) {
	ascii := MonoTheme
	ascii.Glyphs = GlyphsASCII
	for _, c := range []struct {
		th   Theme
		want [3]string
	}{{MonoTheme, [3]string{"┌─ T ───┐", "│ abcde │", "└───────┘"}}, {ascii, [3]string{"+- T ---+", "| abcde |", "+-------+"}}} {
		r := canvas(9, 3, c.th)
		in := r.Frame("T", c.th)
		in.TextStyled(0, 0, "abcde", c.th.Text)
		if got := [3]string{row(r, 0), row(r, 1), row(r, 2)}; got != c.want || in.W != 5 || in.H != 1 {
			t.Errorf("%q, inside %dx%d", got, in.W, in.H)
		}
	}
}

// A rule keeps its title; the hint at its end goes first when both do not fit
func TestRuleKeepsTheTitleBeforeTheHint(t *testing.T) {
	for w, want := range map[int]string{40: "── file · source ────────────── ⏎ edit ─", 24: "── file · source ───────"} {
		r := canvas(w, 1, MonoTheme)
		if r.Rule(0, "file · source", "⏎ edit", MonoTheme); row(r, 0) != want {
			t.Errorf("%d columns: %q", w, row(r, 0))
		}
	}
}

// Wires join where they meet: each cell draws the arms laid through it,
// whatever order they were laid in
func TestWiresJoinWhereTheyMeet(t *testing.T) {
	g := NewWires(5, 3)
	g.V(4, 1, 2)
	g.H(1, 0, 4)
	g.V(2, 0, 2)
	g.H(9, 0, 9) // off the grid
	for line, want := range map[LineType][3]string{LineSingle: {"  │", "──┼─┐", "  │ │"}, LineASCII: {"  |", "--+-+", "  | |"}} {
		r := canvas(5, 3, MonoTheme)
		r.DrawWires(g, line, MonoTheme.Border)
		if got := [3]string{row(r, 0), row(r, 1), row(r, 2)}; got != want {
			t.Errorf("%d: %q, want %q", line, got, want)
		}
	}
}

// A window shows the content's rows from the scroll offset, and fills what
// the content does not reach; a window with no columns draws nothing
func TestWindowShowsTheRowsInView(t *testing.T) {
	r := canvas(6, 3, MonoTheme)
	var v ViewportScroll
	v.SetDimensions(10, r.H)
	v.EnsureRange(7, 1)
	content := func(c Region) {
		for y := range c.H {
			c.TextStyled(0, y, fmt.Sprintf("row %d", y), MonoTheme.Text)
		}
	}
	r.Window(10, &v, MonoTheme.Text, content)
	if got := [3]string{row(r, 0), row(r, 1), row(r, 2)}; got != [3]string{"row 5", "row 6", "row 7"} {
		t.Fatalf("rows 5-7: %q", got)
	}
	r.Window(2, &v, MonoTheme.Text, content)
	if got := [3]string{row(r, 0), row(r, 1), row(r, 2)}; got != [3]string{"row 0", "row 1", ""} {
		t.Fatalf("two rows of content: %q", got)
	}
	r.Sub(8, 0, 4, 3).Window(10, &v, MonoTheme.Text, content) // no columns, past the right edge
}
