package tui

import (
	"github.com/lixenwraith/terminal"
)

// Style bundles foreground, background, and attributes for text rendering
type Style struct {
	Fg   terminal.RGB
	Bg   terminal.RGB
	Attr terminal.Attr
}

// DefaultStyle returns style with zero values (transparent bg)
func DefaultStyle(fg terminal.RGB) Style {
	return Style{Fg: fg}
}

// IsZero returns true if style has no colors or attributes set
func (s Style) IsZero() bool {
	return s.Fg == (terminal.RGB{}) && s.Bg == (terminal.RGB{}) && s.Attr == terminal.AttrNone
}