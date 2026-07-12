package tui

import (
	"github.com/lixenwraith/terminal"
)

// EditorOpts configures editor rendering
type EditorOpts struct {
	LineNumbers  bool
	LineNumWidth int // 0 = auto-size
	WrapLines    bool
	Border       LineType
	Focused      bool
	Style        EditorStyle
}

// EditorStyle defines editor colors
type EditorStyle struct {
	TextFg        terminal.RGB
	TextBg        terminal.RGB
	CursorFg      terminal.RGB
	CursorBg      terminal.RGB
	LineNumFg     terminal.RGB
	LineNumBg     terminal.RGB
	CurrentLineBg terminal.RGB
	BorderFg      terminal.RGB
}

// DefaultEditorStyle returns default colors
func DefaultEditorStyle() EditorStyle {
	return EditorStyle{
		TextFg:        terminal.RGB{R: 220, G: 220, B: 220},
		TextBg:        terminal.RGB{R: 25, G: 25, B: 35},
		CursorFg:      terminal.RGB{R: 0, G: 0, B: 0},
		CursorBg:      terminal.RGB{R: 200, G: 200, B: 200},
		LineNumFg:     terminal.RGB{R: 100, G: 100, B: 120},
		LineNumBg:     terminal.RGB{R: 30, G: 30, B: 40},
		CurrentLineBg: terminal.RGB{R: 35, G: 35, B: 50},
		BorderFg:      terminal.RGB{R: 80, G: 80, B: 100},
	}
}

// Editor renders multi-line editor and returns content height used
func (r Region) Editor(state *EditorState, opts EditorOpts) int {
	if r.W < 3 || r.H < 1 {
		return 0
	}

	style := opts.Style
	if style == (EditorStyle{}) {
		style = DefaultEditorStyle()
	}

	// Calculate content area accounting for border
	contentX := 0
	contentY := 0
	contentW := r.W
	contentH := r.H

	if opts.Border != LineNone {
		if r.H < 3 {
			return 0
		}
		r.Box(opts.Border, style.BorderFg)
		contentX = 1
		contentY = 1
		contentW = r.W - 2
		contentH = r.H - 2
	}

	// Line number gutter
	gutterW := 0
	if opts.LineNumbers {
		gutterW = opts.LineNumWidth
		if gutterW == 0 {
			digits := 1
			n := len(state.Lines)
			for n >= 10 {
				digits++
				n /= 10
			}
			gutterW = digits + 1
		}
		contentW -= gutterW
	}

	if contentW < 1 || contentH < 1 {
		return 0
	}

	state.AdjustScroll(contentW, contentH)

	// Render each visible line
	for y := 0; y < contentH; y++ {
		lineIdx := state.ScrollY + y
		isCurrentLine := lineIdx == state.CursorLine

		bg := style.TextBg
		if isCurrentLine && opts.Focused {
			bg = style.CurrentLineBg
		}

		// Line number gutter
		if opts.LineNumbers {
			for gx := 0; gx < gutterW-1; gx++ {
				r.Cell(contentX+gx, contentY+y, ' ', style.LineNumFg, style.LineNumBg, terminal.AttrNone)
			}
			r.Cell(contentX+gutterW-1, contentY+y, '│', style.LineNumFg, style.LineNumBg, terminal.AttrDim)

			if lineIdx < len(state.Lines) {
				numStr := formatLineNum(lineIdx+1, gutterW-1)
				for i, ch := range numStr {
					r.Cell(contentX+i, contentY+y, ch, style.LineNumFg, style.LineNumBg, terminal.AttrNone)
				}
			}
		}

		textX := contentX + gutterW

		// Fill text area with background
		for x := 0; x < contentW; x++ {
			r.Cell(textX+x, contentY+y, ' ', style.TextFg, bg, terminal.AttrNone)
		}

		if lineIdx >= len(state.Lines) {
			continue
		}

		line := []rune(state.Lines[lineIdx])

		// Scroll indicator left
		if state.ScrollX > 0 && len(line) > 0 {
			r.Cell(textX, contentY+y, '◀', style.LineNumFg, bg, terminal.AttrDim)
		}

		// Render visible text
		for x := 0; x < contentW; x++ {
			charIdx := state.ScrollX + x
			if charIdx >= len(line) {
				break
			}

			ch := line[charIdx]
			fg := style.TextFg
			cellBg := bg

			if opts.Focused && lineIdx == state.CursorLine && charIdx == state.CursorCol {
				fg = style.CursorFg
				cellBg = style.CursorBg
			}

			r.Cell(textX+x, contentY+y, ch, fg, cellBg, terminal.AttrNone)
		}

		// Scroll indicator right
		if state.ScrollX+contentW < len(line) {
			r.Cell(textX+contentW-1, contentY+y, '▶', style.LineNumFg, bg, terminal.AttrDim)
		}

		// Cursor at end of line
		if opts.Focused && lineIdx == state.CursorLine && state.CursorCol >= len(line) {
			cursorX := state.CursorCol - state.ScrollX
			if cursorX >= 0 && cursorX < contentW {
				r.Cell(textX+cursorX, contentY+y, ' ', style.CursorFg, style.CursorBg, terminal.AttrNone)
			}
		}
	}

	// Vertical scroll indicators
	if state.ScrollY > 0 {
		r.Cell(contentX+gutterW+contentW-1, contentY, '▲', style.LineNumFg, style.TextBg, terminal.AttrDim)
	}
	if state.ScrollY+contentH < len(state.Lines) {
		r.Cell(contentX+gutterW+contentW-1, contentY+contentH-1, '▼', style.LineNumFg, style.TextBg, terminal.AttrDim)
	}

	if opts.Border != LineNone {
		return r.H
	}
	return contentH
}

// formatLineNum formats line number right-aligned to width
func formatLineNum(num, width int) string {
	s := ""
	for num > 0 {
		s = string(rune('0'+num%10)) + s
		num /= 10
	}
	if s == "" {
		s = "0"
	}
	for len(s) < width {
		s = " " + s
	}
	return s
}