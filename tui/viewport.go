package tui

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