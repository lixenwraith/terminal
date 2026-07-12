package tui

// --- Scroll position calculation ---

// AdjustScroll returns new scroll offset keeping cursor visible
func AdjustScroll(cursor, scroll, visible, total int) int {
	if total <= visible {
		return 0
	}
	if cursor < scroll {
		return cursor
	}
	if cursor >= scroll+visible {
		return cursor - visible + 1
	}
	return scroll
}

// ScrollPercent returns scroll position as 0-100 percentage
func ScrollPercent(scroll, visible, total int) int {
	if total <= visible {
		return 0
	}
	maxScroll := total - visible
	if maxScroll <= 0 {
		return 0
	}
	pct := (scroll * 100) / maxScroll
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	return pct
}

// --- Clamping utilities ---

// ClampScroll ensures scroll offset is within valid range
func ClampScroll(scroll, visible, total int) int {
	if total <= visible {
		return 0
	}
	maxScroll := total - visible
	if scroll < 0 {
		return 0
	}
	if scroll > maxScroll {
		return maxScroll
	}
	return scroll
}

// ClampCursor ensures cursor is within valid range
func ClampCursor(cursor, total int) int {
	if total <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= total {
		return total - 1
	}
	return cursor
}

// PageDelta returns recommended page scroll amount
func PageDelta(visible int) int {
	delta := visible / 2
	if delta < 1 {
		delta = 1
	}
	return delta
}