package tui

import "github.com/lixenwraith/terminal"

// SparklineChars provides 8-level vertical resolution
var SparklineChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// SparklineOpts configures sparkline rendering
type SparklineOpts struct {
	Min, Max float64 // Range bounds, auto-scale if both 0
	Style    Style
}

// Sparkline renders an inline graph of values, values are mapped to 8-level block characters
func (r Region) Sparkline(x, y, width int, values []float64, opts SparklineOpts) {
	if y < 0 || y >= r.H || width <= 0 || len(values) == 0 {
		return
	}

	// Determine range
	min, max := opts.Min, opts.Max
	if min == 0 && max == 0 {
		min, max = values[0], values[0]
		for _, v := range values {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
	}

	// Handle flat line
	rangeV := max - min
	if rangeV == 0 {
		rangeV = 1
	}

	// Sample or use last N values if more than width
	var sampled []float64
	if len(values) <= width {
		sampled = values
	} else {
		sampled = values[len(values)-width:]
	}

	// Render each value
	for i, v := range sampled {
		if x+i >= r.W {
			break
		}

		// Normalize to 0-1
		norm := (v - min) / rangeV
		if norm < 0 {
			norm = 0
		}
		if norm > 1 {
			norm = 1
		}

		// Map to character index (0-7)
		idx := int(norm * 7.99)
		if idx > 7 {
			idx = 7
		}

		r.Cell(x+i, y, SparklineChars[idx], opts.Style.Fg, opts.Style.Bg, opts.Style.Attr)
	}

	// Pad remaining width with lowest char if values shorter than width
	for i := len(sampled); i < width && x+i < r.W; i++ {
		r.Cell(x+i, y, SparklineChars[0], opts.Style.Fg, opts.Style.Bg, terminal.AttrDim)
	}
}

// SparklineV renders vertical sparkline (bottom to top)
func (r Region) SparklineV(x, y, height int, values []float64, opts SparklineOpts) {
	if x < 0 || x >= r.W || height <= 0 || len(values) == 0 {
		return
	}

	min, max := opts.Min, opts.Max
	if min == 0 && max == 0 {
		min, max = values[0], values[0]
		for _, v := range values {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
	}

	rangeV := max - min
	if rangeV == 0 {
		rangeV = 1
	}

	var sampled []float64
	if len(values) <= height {
		sampled = values
	} else {
		sampled = values[len(values)-height:]
	}

	// Render bottom-up
	for i, v := range sampled {
		yPos := y + height - 1 - i
		if yPos < y || yPos >= r.H {
			continue
		}

		norm := (v - min) / rangeV
		if norm < 0 {
			norm = 0
		}
		if norm > 1 {
			norm = 1
		}

		idx := int(norm * 7.99)
		if idx > 7 {
			idx = 7
		}

		r.Cell(x, yPos, SparklineChars[idx], opts.Style.Fg, opts.Style.Bg, opts.Style.Attr)
	}
}