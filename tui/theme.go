package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Theme styles the widgets that take one. Each role is a Style whose Attr may
// carry color bits (a palette index, the terminal's own color), so one theme
// per color tier, mono included, draws every widget alike. A role with a zero
// Bg and no Bg bits draws on whatever is beneath it.
type Theme struct {
	Text     Style // values and body text; its Bg is the panel's
	Muted    Style // labels, hints, placeholders and defaults, help lines
	Accent   Style // marks: focus, the chosen option, a required field
	Selected Style // the cursor row of a list
	Input    Style // the background of the field being edited
	Cursor   Style // the text cursor
	Error    Style // errors
	Border   Style // boxes and rules
	Glyphs   Glyphs
}

// Glyphs are the marks widgets draw; a selection always changes a glyph, so
// it reads without color. A text console's font takes GlyphsCP437, a
// terminal outside UTF-8 GlyphsASCII.
type Glyphs struct {
	Focus    rune // beside the focused control
	Pointer  rune // the cursor row of a list
	Folded   rune // a closed group
	Unfolded rune // an open group
	On, Off  rune // a toggle
	Required rune // after a required field's label
}

var (
	GlyphsUnicode = Glyphs{Focus: '▌', Pointer: '▸', Folded: '▸', Unfolded: '▾', On: '●', Off: '○', Required: '*'}
	GlyphsCP437   = Glyphs{Focus: '▌', Pointer: '►', Folded: '►', Unfolded: '▼', On: '•', Off: '○', Required: '*'}
	GlyphsASCII   = Glyphs{Focus: '>', Pointer: '>', Folded: '+', Unfolded: '-', On: '*', Off: '-', Required: '*'}
)

// DefaultTheme is a dark theme for true color and 256-color terminals
var DefaultTheme = Theme{
	Text:     Style{Fg: color.RGB{R: 200, G: 200, B: 200}, Bg: color.RGB{R: 20, G: 20, B: 30}},
	Muted:    Style{Fg: color.RGB{R: 120, G: 125, B: 140}},
	Accent:   Style{Fg: color.RGB{R: 80, G: 160, B: 220}, Attr: terminal.AttrBold},
	Selected: Style{Bg: color.RGB{R: 40, G: 45, B: 65}},
	Input:    Style{Bg: color.RGB{R: 30, G: 30, B: 50}},
	Cursor:   Style{Fg: color.RGB{R: 20, G: 20, B: 30}, Bg: color.RGB{R: 200, G: 200, B: 200}},
	Error:    Style{Fg: color.RGB{R: 255, G: 80, B: 80}},
	Border:   Style{Fg: color.RGB{R: 60, G: 80, B: 100}},
	Glyphs:   GlyphsUnicode,
}

// MonoTheme draws with attributes alone in the terminal's own colors: the
// theme for ColorModeNone, and one that reads on any terminal
var MonoTheme = Theme{
	Text:     Style{Attr: own | terminal.AttrBgDefault},
	Muted:    Style{Attr: own | terminal.AttrDim},
	Accent:   Style{Attr: own | terminal.AttrBold},
	Selected: Style{Attr: own | terminal.AttrReverse},
	Input:    Style{Attr: own | terminal.AttrUnderline},
	Cursor:   Style{Attr: own | terminal.AttrReverse},
	Error:    Style{Attr: own | terminal.AttrBold | terminal.AttrUnderline},
	Border:   Style{Attr: own},
	Glyphs:   GlyphsUnicode,
}

// Theme16 uses the ANSI 16 colors alone, in the terminal's own shades, on
// its own background: the theme for ColorMode16, a text console's
var Theme16 = Theme{
	Text:     Style{Attr: own | terminal.AttrBgDefault},
	Muted:    ansi(color.ANSIBrightBlack, 0),
	Accent:   ansi(color.ANSICyan, terminal.AttrBold),
	Selected: Style{Attr: own | terminal.AttrReverse},
	Input:    Style{Attr: own | terminal.AttrUnderline},
	Cursor:   Style{Attr: own | terminal.AttrReverse},
	Error:    ansi(color.ANSIRed, terminal.AttrBold),
	Border:   Style{Attr: own},
	Glyphs:   GlyphsUnicode,
}

// ThemeFor returns the theme for a color mode: DefaultTheme with 256 colors
// or more, Theme16 with 16, MonoTheme with none
func ThemeFor(mode terminal.ColorMode) Theme {
	switch mode {
	case terminal.ColorMode16:
		return Theme16
	case terminal.ColorModeNone:
		return MonoTheme
	}
	return DefaultTheme
}

// own is the terminal's own foreground
const own = terminal.AttrFgDefault

// ansi is a foreground of the ANSI 16, in the terminal's shade of it
func ansi(i uint8, attr terminal.Attr) Style {
	return Style{Fg: color.RGB{R: i}, Attr: terminal.AttrFg256 | attr}
}
