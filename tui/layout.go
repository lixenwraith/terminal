package tui

// --- Centering ---

// Center returns a centered region of given size within outer
func Center(outer Region, w, h int) Region {
	x := (outer.W - w) / 2
	y := (outer.H - h) / 2
	return outer.Sub(x, y, w, h)
}

// --- Ratio-based splitting ---
// Ratios are normalized if they don't sum to 1.0

// SplitH splits region horizontally by ratios (0.0-1.0)
func SplitH(r Region, ratios ...float64) []Region {
	if len(ratios) == 0 {
		return nil
	}

	// Normalize ratios
	var sum float64
	for _, ratio := range ratios {
		sum += ratio
	}
	if sum <= 0 {
		sum = 1
	}

	regions := make([]Region, len(ratios))
	x := 0
	remaining := r.W

	for i, ratio := range ratios {
		var w int
		if i == len(ratios)-1 {
			w = remaining // Last one gets remainder to avoid rounding gaps
		} else {
			w = int((float64(r.W) * ratio / sum) + 0.5) // Round to nearest cell
			if w > remaining {
				w = remaining
			}
		}
		regions[i] = r.Sub(x, 0, w, r.H)
		x += w
		remaining -= w
	}

	return regions
}

// SplitV splits region vertically by ratios (0.0-1.0)
func SplitV(r Region, ratios ...float64) []Region {
	if len(ratios) == 0 {
		return nil
	}

	var sum float64
	for _, ratio := range ratios {
		sum += ratio
	}
	if sum <= 0 {
		sum = 1
	}

	regions := make([]Region, len(ratios))
	y := 0
	remaining := r.H

	for i, ratio := range ratios {
		var h int
		if i == len(ratios)-1 {
			h = remaining
		} else {
			h = int(float64(r.H) * ratio / sum)
		}
		regions[i] = r.Sub(0, y, r.W, h)
		y += h
		remaining -= h
	}

	return regions
}

// --- Fixed-size splitting ---

// SplitHFixed splits with fixed left width, rest to right
func SplitHFixed(r Region, leftW int) (left, right Region) {
	if leftW > r.W {
		leftW = r.W
	}
	if leftW < 0 {
		leftW = 0
	}
	left = r.Sub(0, 0, leftW, r.H)
	right = r.Sub(leftW, 0, r.W-leftW, r.H)
	return
}

// SplitVFixed splits with fixed top height, rest to bottom
func SplitVFixed(r Region, topH int) (top, bottom Region) {
	if topH > r.H {
		topH = r.H
	}
	if topH < 0 {
		topH = 0
	}
	top = r.Sub(0, 0, r.W, topH)
	bottom = r.Sub(0, topH, r.W, r.H-topH)
	return
}

// --- Equal splitting ---

// SplitHEqual splits region into n equal-width columns
// gap specifies spacing between columns (e.g., 1 for divider lines)
// Returns n regions positioned with gaps between them
func SplitHEqual(r Region, n, gap int) []Region {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []Region{r}
	}

	totalGaps := gap * (n - 1)
	availW := r.W - totalGaps
	if availW < n {
		availW = n
	}

	baseW := availW / n
	extra := availW % n

	regions := make([]Region, n)
	x := 0
	for i := 0; i < n; i++ {
		w := baseW
		if i < extra {
			w++
		}
		regions[i] = r.Sub(x, 0, w, r.H)
		x += w + gap
	}
	return regions
}

// SplitVEqual splits region into n equal-height rows
// gap specifies spacing between rows
func SplitVEqual(r Region, n, gap int) []Region {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []Region{r}
	}

	totalGaps := gap * (n - 1)
	availH := r.H - totalGaps
	if availH < n {
		availH = n
	}

	baseH := availH / n
	extra := availH % n

	regions := make([]Region, n)
	y := 0
	for i := 0; i < n; i++ {
		h := baseH
		if i < extra {
			h++
		}
		regions[i] = r.Sub(0, y, r.W, h)
		y += h + gap
	}
	return regions
}

// --- Grid layout ---

// Columns calculates how many columns fit in width
func Columns(availableW, itemW, gap int) int {
	if itemW <= 0 {
		return 0
	}
	if availableW < itemW {
		return 0
	}
	// First item has no gap, subsequent items need gap + itemW
	cols := 1 + (availableW-itemW)/(itemW+gap)
	if cols < 0 {
		cols = 0
	}
	return cols
}

// GridLayout returns a grid of equally sized regions
func GridLayout(r Region, cols, rows, gapX, gapY int) []Region {
	if cols <= 0 || rows <= 0 {
		return nil
	}

	cellW := (r.W - gapX*(cols-1)) / cols
	cellH := (r.H - gapY*(rows-1)) / rows

	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}

	regions := make([]Region, cols*rows)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			x := col * (cellW + gapX)
			y := row * (cellH + gapY)
			regions[row*cols+col] = r.Sub(x, y, cellW, cellH)
		}
	}

	return regions
}

// --- Divider positioning ---

// DividerPositions returns X coordinates for vertical dividers between equal columns
// Use with VLine to draw dividers in gaps created by SplitHEqual
func DividerPositions(regionW, n, gap int) []int {
	if n <= 1 || gap <= 0 {
		return nil
	}

	totalGaps := gap * (n - 1)
	availW := regionW - totalGaps
	if availW < n {
		availW = n
	}

	baseW := availW / n
	extra := availW % n

	positions := make([]int, n-1)
	x := 0
	for i := 0; i < n-1; i++ {
		w := baseW
		if i < extra {
			w++
		}
		x += w
		positions[i] = x
		x += gap
	}
	return positions
}

// HDividerPositions returns Y coordinates for horizontal dividers between equal rows
func HDividerPositions(regionH, n, gap int) []int {
	if n <= 1 || gap <= 0 {
		return nil
	}

	totalGaps := gap * (n - 1)
	availH := regionH - totalGaps
	if availH < n {
		availH = n
	}

	baseH := availH / n
	extra := availH % n

	positions := make([]int, n-1)
	y := 0
	for i := 0; i < n-1; i++ {
		h := baseH
		if i < extra {
			h++
		}
		y += h
		positions[i] = y
		y += gap
	}
	return positions
}

// --- Responsive utilities ---

// FitOrScroll returns true if content exceeds available height
func FitOrScroll(contentH, availableH int) bool {
	return contentH > availableH
}

// Breakpoints should be in descending order

// BreakpointH returns index of first breakpoint <= w, returns len(breakpoints) if w is less than all breakpoints
func BreakpointH(w int, breakpoints ...int) int {
	for i, bp := range breakpoints {
		if w >= bp {
			return i
		}
	}
	return len(breakpoints)
}

// BreakpointV returns index of first breakpoint <= h
func BreakpointV(h int, breakpoints ...int) int {
	return BreakpointH(h, breakpoints...)
}