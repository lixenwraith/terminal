package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Style bundles foreground, background, and attributes for text rendering
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

