package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Button defines a single button in a button bar
type Button struct {
	Label   string
	Key     string // Keyboard hint (e.g., "Ctrl+S")
	Focused bool
}

// ButtonBarOpts configures button bar rendering
type ButtonBarOpts struct {
	Align BarAlign
	Gap   int
	Style ButtonStyle
}

// ButtonStyle defines button bar colors
type ButtonStyle struct {
	LabelFg color.RGB
	LabelBg color.RGB
	KeyFg   color.RGB
	FocusFg color.RGB
	FocusBg color.RGB
	Bg      color.RGB
}

// DefaultButtonStyle returns default button colors with dark background
func DefaultButtonStyle() ButtonStyle {
	return DefaultButtonStyleFrom(color.RGB{R: 25, G: 25, B: 35})
}

// DefaultButtonStyleFrom returns default button colors using the given background
func DefaultButtonStyleFrom(bg color.RGB) ButtonStyle {
	return ButtonStyle{
		LabelFg: color.RGB{R: 200, G: 200, B: 200},
		LabelBg: color.RGB{R: 50, G: 50, B: 60},
		KeyFg:   color.RGB{R: 130, G: 130, B: 150},
		FocusFg: color.RGB{R: 255, G: 255, B: 255},
		FocusBg: color.RGB{R: 60, G: 80, B: 120},
		Bg:      bg,
	}
}

// ButtonBar renders a row of buttons with labels and keyboard hints at row y
func (r Region) ButtonBar(y int, buttons []Button, opts ButtonBarOpts) {
	if len(buttons) == 0 || y < 0 || y >= r.H {
		return
	}

	style := opts.Style
	if style == (ButtonStyle{}) {
		style = DefaultButtonStyle()
	}

	gap := opts.Gap
	if gap < 1 {
		gap = 2
	}

	// Calculate total width
	totalW := 0
	for i, btn := range buttons {
		btnW := RuneLen(btn.Label) + 2
		if btn.Key != "" {
			btnW += RuneLen(btn.Key) + 1
		}
		totalW += btnW
		if i < len(buttons)-1 {
			totalW += gap
		}
	}

	// Starting X
	x := 0
	switch opts.Align {
	case BarAlignRight:
		x = r.W - totalW
	case BarAlignCenter:
		x = (r.W - totalW) / 2
	case BarAlignLeft:
		x = 0
	}
	if x < 0 {
		x = 0
	}

	// Clear row
	for i := 0; i < r.W; i++ {
		r.Cell(i, y, ' ', style.LabelFg, style.Bg, terminal.AttrNone)
	}

	// Render buttons
	for i, btn := range buttons {
		fg := style.LabelFg
		bg := style.LabelBg
		if btn.Focused {
			fg = style.FocusFg
			bg = style.FocusBg
		}

		label := " " + btn.Label + " "
		for _, ch := range label {
			if x >= r.W {
				break
			}
			r.Cell(x, y, ch, fg, bg, terminal.AttrNone)
			x++
		}

		if btn.Key != "" {
			keyStr := " " + btn.Key
			for _, ch := range keyStr {
				if x >= r.W {
					break
				}
				r.Cell(x, y, ch, style.KeyFg, style.Bg, terminal.AttrNone)
				x++
			}
		}

		if i < len(buttons)-1 {
			x += gap
		}
	}
}

