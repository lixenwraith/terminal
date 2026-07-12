package tui

import "github.com/lixenwraith/terminal"

// OverlayStyle specifies overlay appearance
type OverlayStyle uint8

const (
	OverlayFullscreen  OverlayStyle = iota // No border, fills region
	OverlayModal                           // Centered box with border
	OverlayFloating                        // Positioned box with shadow
	OverlayBorderTitle                     // Title embedded in top border line
)

// OverlayOpts configures overlay rendering
type OverlayOpts struct {
	Style   OverlayStyle
	Title   string
	Border  LineType
	Bg      terminal.RGB
	Fg      terminal.RGB // Border and title color
	TitleBg terminal.RGB // Title bar background, zero = same as Fg
	TitleFg terminal.RGB // Title text color, zero = same as Bg

	// Modal/Floating positioning (ignored for Fullscreen)
	Width  int // 0 = 80% of region
	Height int // 0 = 80% of region
	X, Y   int // Offset from center, 0 = centered

	// Shadow for Floating style
	ShadowColor terminal.RGB
}

// DefaultOverlayOpts returns sensible defaults for modal overlay
func DefaultOverlayOpts(title string) OverlayOpts {
	return OverlayOpts{
		Style:   OverlayModal,
		Title:   title,
		Border:  LineDouble,
		Bg:      terminal.RGB{R: 25, G: 25, B: 35},
		Fg:      terminal.RGB{R: 100, G: 140, B: 180},
		TitleBg: terminal.RGB{R: 40, G: 60, B: 90},
		TitleFg: terminal.RGB{R: 255, G: 255, B: 255},
	}
}

// FullscreenOverlayOpts returns opts for fullscreen overlay with title bar
func FullscreenOverlayOpts(title string) OverlayOpts {
	return OverlayOpts{
		Style:   OverlayFullscreen,
		Title:   title,
		Bg:      terminal.RGB{R: 20, G: 20, B: 30},
		Fg:      terminal.RGB{R: 100, G: 140, B: 180},
		TitleBg: terminal.RGB{R: 40, G: 60, B: 90},
		TitleFg: terminal.RGB{R: 255, G: 255, B: 255},
	}
}

// OverlayResult contains regions returned by Overlay rendering
type OverlayResult struct {
	Outer   Region // Full overlay bounds (including border/title)
	Content Region // Inner content area
	TitleY  int    // Y position of title bar in Outer, -1 if no title
}

// Overlay renders an overlay and returns content region for caller to populate
// Caller should render content into result.Content after this call
func (r Region) Overlay(opts OverlayOpts) OverlayResult {
	if r.W < 3 || r.H < 3 {
		return OverlayResult{}
	}

	switch opts.Style {
	case OverlayFullscreen:
		return r.renderFullscreenOverlay(opts)
	case OverlayModal:
		return r.renderModalOverlay(opts)
	case OverlayFloating:
		return r.renderFloatingOverlay(opts)
	case OverlayBorderTitle:
		return r.renderBorderTitleOverlay(opts)
	}

	return OverlayResult{}
}

func (r Region) renderFullscreenOverlay(opts OverlayOpts) OverlayResult {
	r.Fill(opts.Bg)

	result := OverlayResult{
		Outer:  r,
		TitleY: -1,
	}

	contentY := 0
	contentH := r.H

	// Title bar
	if opts.Title != "" {
		titleBg := opts.TitleBg
		if titleBg == (terminal.RGB{}) {
			titleBg = opts.Fg
		}
		titleFg := opts.TitleFg
		if titleFg == (terminal.RGB{}) {
			titleFg = opts.Bg
		}

		// Fill title row
		for x := 0; x < r.W; x++ {
			r.Cell(x, 0, ' ', titleFg, titleBg, terminal.AttrNone)
		}

		// Center title text
		title := opts.Title
		if RuneLen(title) > r.W-4 {
			title = Truncate(title, r.W-4)
		}
		titleX := (r.W - RuneLen(title)) / 2
		r.Text(titleX, 0, title, titleFg, titleBg, terminal.AttrBold)

		result.TitleY = 0
		contentY = 1
		contentH = r.H - 1
	}

	result.Content = r.Sub(0, contentY, r.W, contentH)
	return result
}

func (r Region) renderModalOverlay(opts OverlayOpts) OverlayResult {
	// Calculate dimensions
	w := opts.Width
	if w <= 0 {
		w = r.W * 80 / 100
	}
	h := opts.Height
	if h <= 0 {
		h = r.H * 80 / 100
	}

	// Clamp to region
	if w > r.W-2 {
		w = r.W - 2
	}
	if h > r.H-2 {
		h = r.H - 2
	}
	if w < 5 {
		w = 5
	}
	if h < 3 {
		h = 3
	}

	// Center with offset
	x := (r.W-w)/2 + opts.X
	y := (r.H-h)/2 + opts.Y

	// Clamp position
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+w > r.W {
		x = r.W - w
	}
	if y+h > r.H {
		y = r.H - h
	}

	outer := r.Sub(x, y, w, h)
	outer.BoxFilled(opts.Border, opts.Fg, opts.Bg)

	result := OverlayResult{
		Outer:  outer,
		TitleY: -1,
	}

	// Content inset by border
	contentX := 1
	contentY := 1
	contentW := w - 2
	contentH := h - 2

	// Title in top border
	if opts.Title != "" && contentW > 2 {
		titleBg := opts.TitleBg
		if titleBg == (terminal.RGB{}) {
			titleBg = opts.Fg
		}
		titleFg := opts.TitleFg
		if titleFg == (terminal.RGB{}) {
			titleFg = opts.Bg
		}

		// Fill title row inside border
		for i := 0; i < contentW; i++ {
			outer.Cell(contentX+i, contentY, ' ', titleFg, titleBg, terminal.AttrNone)
		}

		title := opts.Title
		if RuneLen(title) > contentW-2 {
			title = Truncate(title, contentW-2)
		}
		titleX := contentX + (contentW-RuneLen(title))/2
		outer.Text(titleX, contentY, title, titleFg, titleBg, terminal.AttrBold)

		result.TitleY = contentY
		contentY++
		contentH--
	}

	if contentH < 1 {
		contentH = 1
	}

	result.Content = outer.Sub(contentX, contentY, contentW, contentH)
	return result
}

func (r Region) renderFloatingOverlay(opts OverlayOpts) OverlayResult {
	// Same as modal but with shadow
	shadowColor := opts.ShadowColor
	if shadowColor == (terminal.RGB{}) {
		shadowColor = terminal.RGB{R: 10, G: 10, B: 15}
	}

	// Calculate dimensions (same as modal)
	w := opts.Width
	if w <= 0 {
		w = r.W * 80 / 100
	}
	h := opts.Height
	if h <= 0 {
		h = r.H * 80 / 100
	}
	if w > r.W-3 {
		w = r.W - 3
	}
	if h > r.H-2 {
		h = r.H - 2
	}
	if w < 5 {
		w = 5
	}
	if h < 3 {
		h = 3
	}

	x := (r.W-w)/2 + opts.X - 1 // Offset for shadow
	y := (r.H-h)/2 + opts.Y
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+w+1 > r.W {
		x = r.W - w - 1
	}
	if y+h+1 > r.H {
		y = r.H - h - 1
	}

	// Draw shadow
	shadowR := r.Sub(x+1, y+1, w, h)
	shadowR.Fill(shadowColor)

	// Draw box over shadow
	outer := r.Sub(x, y, w, h)
	outer.BoxFilled(opts.Border, opts.Fg, opts.Bg)

	result := OverlayResult{
		Outer:  outer,
		TitleY: -1,
	}

	contentX := 1
	contentY := 1
	contentW := w - 2
	contentH := h - 2

	if opts.Title != "" && contentW > 2 {
		titleBg := opts.TitleBg
		if titleBg == (terminal.RGB{}) {
			titleBg = opts.Fg
		}
		titleFg := opts.TitleFg
		if titleFg == (terminal.RGB{}) {
			titleFg = opts.Bg
		}

		for i := 0; i < contentW; i++ {
			outer.Cell(contentX+i, contentY, ' ', titleFg, titleBg, terminal.AttrNone)
		}

		title := opts.Title
		if RuneLen(title) > contentW-2 {
			title = Truncate(title, contentW-2)
		}
		titleX := contentX + (contentW-RuneLen(title))/2
		outer.Text(titleX, contentY, title, titleFg, titleBg, terminal.AttrBold)

		result.TitleY = contentY
		contentY++
		contentH--
	}

	if contentH < 1 {
		contentH = 1
	}

	result.Content = outer.Sub(contentX, contentY, contentW, contentH)
	return result
}

func (r Region) renderBorderTitleOverlay(opts OverlayOpts) OverlayResult {
	r.BoxFilled(opts.Border, opts.Fg, opts.Bg)

	result := OverlayResult{
		Outer:  r,
		TitleY: 0,
	}

	if opts.Title != "" && r.W > 6 {
		titleFg := opts.TitleFg
		if titleFg == (terminal.RGB{}) {
			titleFg = opts.Fg
		}
		title := " " + opts.Title + " "
		if RuneLen(title) > r.W-4 {
			title = Truncate(title, r.W-4)
		}
		titleX := (r.W - RuneLen(title)) / 2
		r.Text(titleX, 0, title, titleFg, opts.Bg, terminal.AttrBold)
	}

	result.Content = r.Sub(1, 1, r.W-2, r.H-2)
	return result
}

// OverlayState manages overlay visibility and content state
type OverlayState struct {
	Visible bool
	Opts    OverlayOpts
	Data    any // Application-specific state
}

// NewOverlayState creates hidden overlay state
func NewOverlayState(opts OverlayOpts) *OverlayState {
	return &OverlayState{
		Visible: false,
		Opts:    opts,
	}
}

// Show makes overlay visible
func (o *OverlayState) Show() {
	o.Visible = true
}

// Hide makes overlay invisible
func (o *OverlayState) Hide() {
	o.Visible = false
}

// Toggle switches visibility
func (o *OverlayState) Toggle() {
	o.Visible = !o.Visible
}