package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// BarSection represents one segment of a status bar
type BarSection struct {
	Label      string
	Value      string
	LabelStyle Style
	ValueStyle Style
	Priority   int // Higher = survives truncation
}

// BarAlign specifies status bar alignment mode
type BarAlign uint8

const (
	BarAlignRight      BarAlign = iota // Pack sections from right
	BarAlignLeft                       // Pack sections from left
	BarAlignDistribute                 // Evenly space sections
	BarAlignCenter                     // Center all sections as a group
)

// BarOpts configures status bar rendering
type BarOpts struct {
	Separator string // Between sections, default " │ "
	SepStyle  Style  // Separator styling
	Bg        color.RGB
	Align     BarAlign
	Padding   int // Left/right padding, default 1
}

// DefaultBarOpts returns sensible defaults
func DefaultBarOpts() BarOpts {
	return BarOpts{
		Separator: " │ ",
		SepStyle:  Style{Fg: color.RGB{R: 80, G: 80, B: 100}},
		Padding:   1,
		Align:     BarAlignRight,
	}
}

// StatusBar renders horizontal status bar on row y
func (r Region) StatusBar(y int, sections []BarSection, opts BarOpts) {
	if y < 0 || y >= r.H || len(sections) == 0 {
		return
	}

	if opts.Separator == "" {
		opts.Separator = " │ "
	}
	if opts.Padding == 0 {
		opts.Padding = 1
	}

	// Fill background
	for x := 0; x < r.W; x++ {
		r.Cell(x, y, ' ', color.RGB{}, opts.Bg, terminal.AttrNone)
	}

	sepLen := RuneLen(opts.Separator)

	// Calculate total width needed
	totalW := 0
	sectionWidths := make([]int, len(sections))
	for i, sec := range sections {
		w := RuneLen(sec.Label) + RuneLen(sec.Value)
		sectionWidths[i] = w
		totalW += w
		if i < len(sections)-1 {
			totalW += sepLen
		}
	}

	availW := r.W - opts.Padding*2

	// Truncate low-priority sections if needed
	if totalW > availW {
		sections, sectionWidths = truncateSections(sections, sectionWidths, sepLen, availW)
		totalW = 0
		for i, w := range sectionWidths {
			totalW += w
			if i < len(sectionWidths)-1 {
				totalW += sepLen
			}
		}
	}

	// Calculate starting position based on alignment
	var x int
	switch opts.Align {
	case BarAlignLeft:
		x = opts.Padding
	case BarAlignRight:
		x = r.W - opts.Padding - totalW
		if x < opts.Padding {
			x = opts.Padding
		}
	case BarAlignDistribute:
		x = opts.Padding
		// Handled specially below
	case BarAlignCenter:
		x = (r.W - totalW) / 2
		if x < opts.Padding {
			x = opts.Padding
		}
	}

	// Render sections
	if opts.Align == BarAlignDistribute && len(sections) > 1 {
		gap := (availW - totalW) / (len(sections) - 1)
		if gap < 0 {
			gap = 0
		}
		for i, sec := range sections {
			x = r.renderBarSection(x, y, sec, opts)
			if i < len(sections)-1 {
				x += gap
			}
		}
	} else {
		for i, sec := range sections {
			x = r.renderBarSection(x, y, sec, opts)
			if i < len(sections)-1 {
				// Separator
				for j, ch := range opts.Separator {
					if x+j < r.W-opts.Padding {
						r.Cell(x+j, y, ch, opts.SepStyle.Fg, opts.Bg, opts.SepStyle.Attr)
					}
				}
				x += sepLen
			}
		}
	}
}

func (r Region) renderBarSection(x, y int, sec BarSection, opts BarOpts) int {
	// Label
	for _, ch := range sec.Label {
		if x >= r.W-opts.Padding {
			break
		}
		r.Cell(x, y, ch, sec.LabelStyle.Fg, opts.Bg, sec.LabelStyle.Attr)
		x++
	}
	// Value
	for _, ch := range sec.Value {
		if x >= r.W-opts.Padding {
			break
		}
		r.Cell(x, y, ch, sec.ValueStyle.Fg, opts.Bg, sec.ValueStyle.Attr)
		x++
	}
	return x
}

// truncateSections removes lowest priority sections until fit
func truncateSections(sections []BarSection, widths []int, sepLen, availW int) ([]BarSection, []int) {
	// Copy to avoid modifying original
	secs := make([]BarSection, len(sections))
	copy(secs, sections)
	ws := make([]int, len(widths))
	copy(ws, widths)

	for {
		total := 0
		for i, w := range ws {
			total += w
			if i < len(ws)-1 {
				total += sepLen
			}
		}
		if total <= availW || len(secs) <= 1 {
			break
		}

		// Find lowest priority
		minIdx := 0
		minPrio := secs[0].Priority
		for i, sec := range secs {
			if sec.Priority < minPrio {
				minPrio = sec.Priority
				minIdx = i
			}
		}

		// RemoveEntityAt it
		secs = append(secs[:minIdx], secs[minIdx+1:]...)
		ws = append(ws[:minIdx], ws[minIdx+1:]...)
	}

	return secs, ws
}

// QuickStatusBar renders simple label:value pairs right-aligned
func (r Region) QuickStatusBar(y int, pairs [][2]string, labelFg, valueFg, bg color.RGB) {
	sections := make([]BarSection, len(pairs))
	for i, p := range pairs {
		sections[i] = BarSection{
			Label:      p[0],
			Value:      p[1],
			LabelStyle: Style{Fg: labelFg},
			ValueStyle: Style{Fg: valueFg},
		}
	}
	r.StatusBar(y, sections, BarOpts{
		Bg:    bg,
		Align: BarAlignRight,
	})
}
