package tui

import "github.com/lixenwraith/terminal"

// ViewportScroll manages row-based scroll for content regions
// Distinct from ScrollState which is item-index based
type ViewportScroll struct {
	Offset    int // Row offset from top of content
	ContentH  int // Total content height in rows
	ViewportH int // Visible viewport height
}

// NewViewportScroll creates viewport scroll state
func NewViewportScroll() *ViewportScroll {
	return &ViewportScroll{}
}

// SetDimensions updates content and viewport heights, clamps offset
func (v *ViewportScroll) SetDimensions(contentH, viewportH int) {
	v.ContentH = contentH
	v.ViewportH = viewportH
	v.clamp()
}

// MaxOffset returns maximum valid scroll offset
func (v *ViewportScroll) MaxOffset() int {
	maxOffset := v.ContentH - v.ViewportH
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

// CanScroll returns true if content exceeds viewport
func (v *ViewportScroll) CanScroll() bool {
	return v.ContentH > v.ViewportH
}

// ScrollBy adjusts offset by delta
func (v *ViewportScroll) ScrollBy(delta int) {
	v.Offset += delta
	v.clamp()
}

// ScrollTo sets absolute offset
func (v *ViewportScroll) ScrollTo(pos int) {
	v.Offset = pos
	v.clamp()
}

// PageUp scrolls up by viewport height
func (v *ViewportScroll) PageUp() {
	v.ScrollBy(-v.ViewportH)
}

// PageDown scrolls down by viewport height
func (v *ViewportScroll) PageDown() {
	v.ScrollBy(v.ViewportH)
}

// Home scrolls to top
func (v *ViewportScroll) Home() {
	v.Offset = 0
}

// End scrolls to bottom
func (v *ViewportScroll) End() {
	v.Offset = v.MaxOffset()
}

func (v *ViewportScroll) clamp() {
	max := v.MaxOffset()
	if v.Offset > max {
		v.Offset = max
	}
	if v.Offset < 0 {
		v.Offset = 0
	}
}

// IsVisible returns true if content row range intersects viewport
func (v *ViewportScroll) IsVisible(y, h int) bool {
	return y+h > v.Offset && y < v.Offset+v.ViewportH
}

// ClipToViewport maps content coordinates to viewport coordinates
// Returns viewY (in viewport), viewH (visible height), contentOffset (rows clipped from top)
// visible=false if entirely outside viewport
func (v *ViewportScroll) ClipToViewport(y, h int) (viewY, viewH, contentOffset int, visible bool) {
	if !v.IsVisible(y, h) {
		return 0, 0, 0, false
	}

	viewY = y - v.Offset
	viewH = h
	contentOffset = 0

	if viewY < 0 {
		contentOffset = -viewY
		viewH += viewY
		viewY = 0
	}

	if viewY+viewH > v.ViewportH {
		viewH = v.ViewportH - viewY
	}

	return viewY, viewH, contentOffset, viewH > 0
}

// EnsureRange scrolls the least distance that brings a content row range into
// view; a range taller than the viewport is top-aligned
func (v *ViewportScroll) EnsureRange(y, h int) {
	switch {
	case h >= v.ViewportH || y < v.Offset:
		v.Offset = y
	case y+h > v.Offset+v.ViewportH:
		v.Offset = y + h - v.ViewportH
	}
	v.clamp()
}

// Window shows content h rows tall through the region, from v's offset:
// draw paints a buffer as wide as the region and h tall, filled with bg, and
// the rows in view are copied. Window sets v's heights; a caller keeping a
// row in view sets them first and calls EnsureRange.
func (r Region) Window(h int, v *ViewportScroll, bg Style, draw func(Region)) {
	h = max(0, h)
	v.SetDimensions(h, r.H)
	if r.W <= 0 || r.H <= 0 {
		return
	}
	cells := make([]terminal.Cell, r.W*h)
	full := NewRegion(cells, r.W, 0, 0, r.W, h)
	full.FillStyle(bg)
	draw(full)
	shown := min(r.H, h-v.Offset)
	for y := range shown {
		copy(r.Cells[(r.Y+y)*r.TotalW+r.X:][:r.W], cells[(v.Offset+y)*r.W:][:r.W])
	}
	r.Sub(0, shown, r.W, r.H-shown).FillStyle(bg)
}
