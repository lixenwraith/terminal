package tui

// ScrollState tracks scroll position for a scrollable container
type ScrollState struct {
	Offset    int // First visible item index
	Total     int // Total item count
	Visible   int // Visible item count (viewport height)
	Selection int // Currently selected item, -1 if none
}

// NewScrollState creates initialized scroll state
func NewScrollState(total, visible int) *ScrollState {
	return &ScrollState{
		Total:     total,
		Visible:   visible,
		Selection: -1,
	}
}

// --- Scroll manipulation ---

// ScrollBy adjusts offset by delta, clamping to valid range
func (s *ScrollState) ScrollBy(delta int) {
	s.Offset += delta
	s.Clamp()
}

// ScrollTo sets offset to specific position
func (s *ScrollState) ScrollTo(pos int) {
	s.Offset = pos
	s.Clamp()
}

// EnsureVisible adjusts offset to make item at pos visible
func (s *ScrollState) EnsureVisible(pos int) {
	if pos < s.Offset {
		s.Offset = pos
	} else if pos >= s.Offset+s.Visible {
		s.Offset = pos - s.Visible + 1
	}
	s.Clamp()
}

// Clamp ensures offset is within valid range
func (s *ScrollState) Clamp() {
	s.Offset = ClampScroll(s.Offset, s.Visible, s.Total)
}

// --- Page navigation ---

// PageUp scrolls up by half visible height
func (s *ScrollState) PageUp() {
	s.ScrollBy(-PageDelta(s.Visible))
}

// PageDown scrolls down by half visible height
func (s *ScrollState) PageDown() {
	s.ScrollBy(PageDelta(s.Visible))
}

// --- State updates ---

// SetTotal updates total count and reclamps
func (s *ScrollState) SetTotal(total int) {
	s.Total = total
	s.Clamp()
	if s.Selection >= total {
		s.Selection = total - 1
	}
}

// SetVisible updates visible count and reclamps
func (s *ScrollState) SetVisible(visible int) {
	s.Visible = visible
	s.Clamp()
}

// --- Positions queries ---

// AtTop returns true if scrolled to top
func (s *ScrollState) AtTop() bool {
	return s.Offset == 0
}

// AtBottom returns true if scrolled to bottom
func (s *ScrollState) AtBottom() bool {
	if s.Total <= s.Visible {
		return true
	}
	return s.Offset >= s.Total-s.Visible
}

// --- Selection management ---

// Select sets selection and ensures it's visible
func (s *ScrollState) Select(idx int) {
	s.Selection = ClampCursor(idx, s.Total)
	s.EnsureVisible(s.Selection)
}

// SelectNext moves selection down
func (s *ScrollState) SelectNext() {
	if s.Selection < s.Total-1 {
		s.Selection++
		s.EnsureVisible(s.Selection)
	}
}

// SelectPrev moves selection up
func (s *ScrollState) SelectPrev() {
	if s.Selection > 0 {
		s.Selection--
		s.EnsureVisible(s.Selection)
	}
}