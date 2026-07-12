package tui

// TreeState manages navigation state for a tree
type TreeState struct {
	Cursor  int
	Scroll  int
	Visible int // Viewport height
}

// NewTreeState creates initialized tree state
func NewTreeState(visible int) *TreeState {
	return &TreeState{
		Visible: visible,
	}
}

// --- Cursor movement ---

// MoveCursor adjusts cursor position by delta
func (t *TreeState) MoveCursor(delta, total int) {
	t.Cursor += delta
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	if t.Cursor >= total {
		t.Cursor = total - 1
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	t.AdjustScroll(total)
}

// AdjustScroll ensures cursor is visible
func (t *TreeState) AdjustScroll(total int) {
	if t.Visible <= 0 {
		return
	}
	if t.Cursor < t.Scroll {
		t.Scroll = t.Cursor
	}
	if t.Cursor >= t.Scroll+t.Visible {
		t.Scroll = t.Cursor - t.Visible + 1
	}
	// Clamp scroll
	maxScroll := total - t.Visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if t.Scroll > maxScroll {
		t.Scroll = maxScroll
	}
	if t.Scroll < 0 {
		t.Scroll = 0
	}
}

// --- Jump navigation ---

// JumpStart moves cursor to first item
func (t *TreeState) JumpStart() {
	t.Cursor = 0
	t.Scroll = 0
}

// JumpEnd moves cursor to last item
func (t *TreeState) JumpEnd(total int) {
	if total > 0 {
		t.Cursor = total - 1
	}
	t.AdjustScroll(total)
}

// --- Page navigation ---

// PageUp scrolls up by half viewport
func (t *TreeState) PageUp(total int) {
	delta := t.Visible / 2
	if delta < 1 {
		delta = 1
	}
	t.MoveCursor(-delta, total)
}

// PageDown scrolls down by half viewport
func (t *TreeState) PageDown(total int) {
	delta := t.Visible / 2
	if delta < 1 {
		delta = 1
	}
	t.MoveCursor(delta, total)
}

// SetVisible updates viewport height
func (t *TreeState) SetVisible(visible int) {
	t.Visible = visible
}