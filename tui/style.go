package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Style bundles foreground, background, and attributes for text rendering.
// Attr's color bits say how to read Fg and Bg: terminal.AttrFg256 a palette
// index in R, terminal.AttrFgDefault the terminal's own color.
type Style struct {
	Fg   color.RGB
	Bg   color.RGB
	Attr terminal.Attr
}

// DefaultStyle returns style with zero values (transparent bg)
func DefaultStyle(fg color.RGB) Style {
	return Style{Fg: fg}
}

// IsZero returns true if style has no colors or attributes set
func (s Style) IsZero() bool {
	return s.Fg == (color.RGB{}) && s.Bg == (color.RGB{}) && s.Attr == terminal.AttrNone
}

// On returns s drawn on bg: s's foreground, bg's background (s's own where
// bg's is transparent), and the attributes of both
func (s Style) On(bg Style) Style {
	out := Style{Fg: s.Fg, Bg: s.Bg, Attr: s.Attr | bg.Attr&terminal.AttrStyle}
	if bg.Bg != (color.RGB{}) || bg.Attr&terminal.AttrBgColor != 0 {
		out.Bg, out.Attr = bg.Bg, out.Attr&^terminal.AttrBgColor|bg.Attr&terminal.AttrBgColor
	}
	return out
}
