package tui

import "github.com/lixenwraith/terminal"

// TabBounds stores position and size of a rendered tab
type TabBounds struct {
	X, W int
}

// TabBarOpts configures tab bar rendering
type TabBarOpts struct {
	ActiveStyle   Style
	InactiveStyle Style
	Separator     string // Between tabs, default " │ "
	Padding       int    // Horizontal padding inside each tab, default 1
}

// DefaultTabBarOpts returns sensible defaults
func DefaultTabBarOpts() TabBarOpts {
	return TabBarOpts{
		ActiveStyle:   Style{Attr: terminal.AttrBold | terminal.AttrReverse},
		InactiveStyle: Style{Attr: terminal.AttrNone},
		Separator:     " │ ",
		Padding:       1,
	}
}

// TabBar renders horizontal tab strip at row y
// Returns bounds of each tab for hit testing / navigation
func (r Region) TabBar(y int, titles []string, active int, opts TabBarOpts) []TabBounds {
	if y < 0 || y >= r.H || len(titles) == 0 {
		return nil
	}

	if opts.Separator == "" {
		opts.Separator = " │ "
	}

	bounds := make([]TabBounds, len(titles))
	x := 0
	sepLen := RuneLen(opts.Separator)

	for i, title := range titles {
		if x >= r.W {
			break
		}

		// Calculate tab width
		tabW := RuneLen(title) + opts.Padding*2
		if x+tabW > r.W {
			tabW = r.W - x
		}

		bounds[i] = TabBounds{X: x, W: tabW}

		// Select style
		style := opts.InactiveStyle
		if i == active {
			style = opts.ActiveStyle
		}

		// Render padding + title + padding
		for j := 0; j < opts.Padding && x+j < r.W; j++ {
			r.Cell(x+j, y, ' ', style.Fg, style.Bg, style.Attr)
		}

		titleStart := x + opts.Padding
		for j, ch := range title {
			if titleStart+j >= r.W {
				break
			}
			r.Cell(titleStart+j, y, ch, style.Fg, style.Bg, style.Attr)
		}

		for j := 0; j < opts.Padding; j++ {
			pos := x + opts.Padding + RuneLen(title) + j
			if pos < r.W {
				r.Cell(pos, y, ' ', style.Fg, style.Bg, style.Attr)
			}
		}

		x += tabW

		// Separator between tabs
		if i < len(titles)-1 && x+sepLen <= r.W {
			for j, ch := range opts.Separator {
				r.Cell(x+j, y, ch, opts.InactiveStyle.Fg, opts.InactiveStyle.Bg, terminal.AttrDim)
			}
			x += sepLen
		}
	}

	return bounds
}

// TabBarCentered renders tab bar centered horizontally
func (r Region) TabBarCentered(y int, titles []string, active int, opts TabBarOpts) []TabBounds {
	if len(titles) == 0 {
		return nil
	}

	if opts.Separator == "" {
		opts.Separator = " │ "
	}

	// Calculate total width
	totalW := 0
	sepLen := RuneLen(opts.Separator)
	for i, title := range titles {
		totalW += RuneLen(title) + opts.Padding*2
		if i < len(titles)-1 {
			totalW += sepLen
		}
	}

	// Create sub-region for centering
	startX := (r.W - totalW) / 2
	if startX < 0 {
		startX = 0
	}

	subR := r.Sub(startX, y, totalW, 1)
	bounds := subR.TabBar(0, titles, active, opts)

	// Adjust bounds to parent coordinates
	for i := range bounds {
		bounds[i].X += startX
	}

	return bounds
}