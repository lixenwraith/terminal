package tui

// MasonryItem represents a single item in masonry layout
type MasonryItem struct {
	Key    string
	Height int
	Data   any
}

// MasonryLayout holds calculated position for an item
type MasonryLayout struct {
	X, Y, W, H int
	Item       MasonryItem
}

// MasonryOpts configures masonry layout
type MasonryOpts struct {
	Columns     int
	GapX        int // Columns between masonry columns
	GapY        int // Rows between stacked items
	MinColW     int
	Breakpoints map[int]int
}

// DefaultMasonryOpts returns sensible defaults
func DefaultMasonryOpts() MasonryOpts {
	return MasonryOpts{
		GapX:    2,
		GapY:    1,
		MinColW: 30,
		Breakpoints: map[int]int{
			140: 4,
			100: 3,
			60:  2,
		},
	}
}

// MasonryState manages masonry layout with viewport scroll
type MasonryState struct {
	Viewport *ViewportScroll
	Layouts  []MasonryLayout
}

// NewMasonryState creates masonry state
func NewMasonryState() *MasonryState {
	return &MasonryState{
		Viewport: NewViewportScroll(),
	}
}

// CalculateLayout computes item positions
func (m *MasonryState) CalculateLayout(items []MasonryItem, width int, opts MasonryOpts) {
	cols := opts.Columns
	if cols <= 0 {
		cols = m.autoColumns(width, opts)
	}

	gapX := max(opts.GapX, 0)
	gapY := max(opts.GapY, 0)

	colW := max((width-(cols-1)*gapX)/cols, 1)

	m.Layouts = make([]MasonryLayout, 0, len(items))
	colHeights := make([]int, cols)

	for _, item := range items {
		minCol := 0
		minH := colHeights[0]
		for i := 1; i < cols; i++ {
			if colHeights[i] < minH {
				minH = colHeights[i]
				minCol = i
			}
		}

		x := minCol * (colW + gapX)
		y := colHeights[minCol]

		m.Layouts = append(m.Layouts, MasonryLayout{
			X: x, Y: y, W: colW, H: item.Height,
			Item: item,
		})

		colHeights[minCol] += item.Height + gapY
	}

	totalH := 0
	for _, h := range colHeights {
		if h > totalH {
			totalH = h
		}
	}
	if totalH > 0 {
		totalH -= gapY
	}

	m.Viewport.ContentH = totalH
}

func (m *MasonryState) autoColumns(width int, opts MasonryOpts) int {
	if opts.Breakpoints != nil {
		best := 1
		bestThresh := 0
		for thresh, cols := range opts.Breakpoints {
			if width >= thresh && thresh > bestThresh {
				best = cols
				bestThresh = thresh
			}
		}
		return best
	}

	cols := max(width/opts.MinColW, 1)
	return cols
}

// SetViewport updates viewport height
func (m *MasonryState) SetViewport(h int) {
	m.Viewport.ViewportH = h
	m.Viewport.clamp()
}

// MasonryRenderFunc renders a single item
type MasonryRenderFunc func(region Region, layout MasonryLayout, contentOffset int)

// Masonry renders visible items via callback
func (r Region) Masonry(state *MasonryState, render MasonryRenderFunc) {
	state.SetViewport(r.H)

	for _, l := range state.Layouts {
		viewY, viewH, offset, visible := state.Viewport.ClipToViewport(l.Y, l.H)
		if !visible {
			continue
		}

		itemRegion := r.Sub(l.X, viewY, l.W, viewH)
		render(itemRegion, l, offset)
	}
}

// ScrollIndicator renders scroll position indicator for masonry
func (m *MasonryState) ScrollIndicator() string {
	if !m.Viewport.CanScroll() {
		return ""
	}
	pos := m.Viewport.Offset + 1
	maxPos := m.Viewport.MaxOffset() + 1
	return "[" + itoa(pos) + "/" + itoa(maxPos) + "]"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
