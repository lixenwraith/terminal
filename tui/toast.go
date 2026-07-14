package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ToastPosition specifies where toast renders
type ToastPosition uint8

const (
	ToastBottom      ToastPosition = iota // Full-width bar at bottom
	ToastTop                              // Full-width bar at top
	ToastBottomRight                      // Floating box bottom-right
	ToastBottomLeft                       // Floating box bottom-left
	ToastTopRight                         // Floating box top-right
	ToastTopLeft                          // Floating box top-left
	ToastCenter                           // Centered floating box
)

// ToastSeverity defines message type for styling
type ToastSeverity uint8

const (
	ToastInfo    ToastSeverity = iota // Default, neutral
	ToastSuccess                      // Green, positive
	ToastWarning                      // Yellow, caution
	ToastError                        // Red, failure
)

// ToastStyle defines visual appearance
type ToastStyle uint8

const (
	ToastStyleMinimal ToastStyle = iota // No border, just text
	ToastStyleBar                       // Full-width background bar
	ToastStyleBox                       // Bordered box
	ToastStyleRounded                   // Rounded border box
	ToastStyleDouble                    // Double-line border
	ToastStyleShadow                    // Box with shadow effect
)

// ToastIcons for severity levels
var ToastIcons = map[ToastSeverity]rune{
	ToastInfo:    'ℹ',
	ToastSuccess: '✓',
	ToastWarning: '⚠',
	ToastError:   '✗',
}

// ToastColors default colors per severity
var ToastColors = map[ToastSeverity]struct{ Fg, Bg, Icon color.RGB }{
	ToastInfo: {
		Fg:   color.RGB{R: 200, G: 200, B: 200},
		Bg:   color.RGB{R: 40, G: 40, B: 50},
		Icon: color.RGB{R: 100, G: 150, B: 255},
	},
	ToastSuccess: {
		Fg:   color.RGB{R: 220, G: 255, B: 220},
		Bg:   color.RGB{R: 30, G: 60, B: 30},
		Icon: color.RGB{R: 80, G: 220, B: 80},
	},
	ToastWarning: {
		Fg:   color.RGB{R: 255, G: 240, B: 200},
		Bg:   color.RGB{R: 60, G: 50, B: 20},
		Icon: color.RGB{R: 255, G: 200, B: 60},
	},
	ToastError: {
		Fg:   color.RGB{R: 255, G: 220, B: 220},
		Bg:   color.RGB{R: 60, G: 25, B: 25},
		Icon: color.RGB{R: 255, G: 80, B: 80},
	},
}

// ToastOpts configures toast rendering
type ToastOpts struct {
	Message    string
	Severity   ToastSeverity
	Position   ToastPosition
	Style      ToastStyle
	ShowIcon   bool
	MinWidth   int // Minimum width for floating toasts, 0 = auto
	MaxWidth   int // Maximum width, 0 = region width
	Padding    int // Horizontal padding, default 1
	MarginX    int // Margin from edge for floating positions
	MarginY    int // Margin from edge for floating positions
	CustomFg   color.RGB
	CustomBg   color.RGB
	CustomIcon color.RGB
}

// DefaultToastOpts returns sensible defaults
func DefaultToastOpts(message string, severity ToastSeverity) ToastOpts {
	return ToastOpts{
		Message:  message,
		Severity: severity,
		Position: ToastBottom,
		Style:    ToastStyleBar,
		ShowIcon: true,
		Padding:  1,
		MarginX:  2,
		MarginY:  1,
	}
}

// Toast renders a toast message overlay
// Returns the region occupied by the toast for hit testing
func (r Region) Toast(opts ToastOpts) Region {
	if r.W < 5 || r.H < 1 || opts.Message == "" {
		return Region{}
	}

	// Resolve colors
	fg, bg, iconFg := opts.CustomFg, opts.CustomBg, opts.CustomIcon
	if fg == (color.RGB{}) {
		fg = ToastColors[opts.Severity].Fg
	}
	if bg == (color.RGB{}) {
		bg = ToastColors[opts.Severity].Bg
	}
	if iconFg == (color.RGB{}) {
		iconFg = ToastColors[opts.Severity].Icon
	}

	padding := opts.Padding
	if padding == 0 {
		padding = 1
	}

	// Calculate content width
	iconW := 0
	if opts.ShowIcon {
		iconW = 2 // icon + space
	}
	msgLen := RuneLen(opts.Message)
	contentW := iconW + msgLen + padding*2

	// Determine toast dimensions based on style
	borderW := 0
	if opts.Style >= ToastStyleBox {
		borderW = 2
	}

	toastW := contentW + borderW
	toastH := 1 + borderW

	// Apply width constraints
	maxW := opts.MaxWidth
	if maxW == 0 || maxW > r.W {
		maxW = r.W
	}
	if toastW > maxW {
		toastW = maxW
	}
	if opts.MinWidth > 0 && toastW < opts.MinWidth {
		toastW = opts.MinWidth
	}

	// Calculate position
	var toastX, toastY int
	marginX := opts.MarginX
	marginY := opts.MarginY

	switch opts.Position {
	case ToastBottom:
		toastX = 0
		toastY = r.H - toastH
		toastW = r.W // Full width for bar positions
	case ToastTop:
		toastX = 0
		toastY = 0
		toastW = r.W
	case ToastBottomRight:
		toastX = r.W - toastW - marginX
		toastY = r.H - toastH - marginY
	case ToastBottomLeft:
		toastX = marginX
		toastY = r.H - toastH - marginY
	case ToastTopRight:
		toastX = r.W - toastW - marginX
		toastY = marginY
	case ToastTopLeft:
		toastX = marginX
		toastY = marginY
	case ToastCenter:
		toastX = (r.W - toastW) / 2
		toastY = (r.H - toastH) / 2
	}

	// Clamp position
	if toastX < 0 {
		toastX = 0
	}
	if toastY < 0 {
		toastY = 0
	}

	toastRegion := r.Sub(toastX, toastY, toastW, toastH)

	// Render based on style
	switch opts.Style {
	case ToastStyleMinimal:
		r.renderToastContent(toastRegion, opts, fg, bg, iconFg, 0)

	case ToastStyleBar:
		toastRegion.Fill(bg)
		r.renderToastContent(toastRegion, opts, fg, bg, iconFg, 0)

	case ToastStyleBox:
		toastRegion.BoxFilled(LineSingle, fg, bg)
		r.renderToastContent(toastRegion.Inset(1), opts, fg, bg, iconFg, 0)

	case ToastStyleRounded:
		toastRegion.BoxFilled(LineRounded, fg, bg)
		r.renderToastContent(toastRegion.Inset(1), opts, fg, bg, iconFg, 0)

	case ToastStyleDouble:
		toastRegion.BoxFilled(LineDouble, fg, bg)
		r.renderToastContent(toastRegion.Inset(1), opts, fg, bg, iconFg, 0)

	case ToastStyleShadow:
		// Shadow offset
		shadowRegion := r.Sub(toastX+1, toastY+1, toastW, toastH)
		shadowRegion.Fill(color.RGB{R: 10, G: 10, B: 15})
		toastRegion.BoxFilled(LineSingle, fg, bg)
		r.renderToastContent(toastRegion.Inset(1), opts, fg, bg, iconFg, 0)
	}

	return toastRegion
}

func (r Region) renderToastContent(content Region, opts ToastOpts, fg, bg, iconFg color.RGB, _ int) {
	if content.W < 1 || content.H < 1 {
		return
	}

	x := opts.Padding
	y := 0

	// Icon
	if opts.ShowIcon {
		icon := ToastIcons[opts.Severity]
		if x < content.W {
			content.Cell(x, y, icon, iconFg, bg, terminal.AttrBold)
		}
		x += 2
	}

	// Message
	msg := opts.Message
	availW := content.W - x - opts.Padding
	if availW < 1 {
		return
	}
	if RuneLen(msg) > availW {
		msg = Truncate(msg, availW)
	}
	content.Text(x, y, msg, fg, bg, terminal.AttrNone)
}

// ToastState manages toast lifecycle
type ToastState struct {
	Visible    bool
	Opts       ToastOpts
	FramesLeft int // Countdown to auto-dismiss, -1 = persistent
}

// NewToastState creates a toast that auto-dismisses after frames
// Use frames=-1 for persistent toast
func NewToastState(opts ToastOpts, frames int) *ToastState {
	return &ToastState{
		Visible:    true,
		Opts:       opts,
		FramesLeft: frames,
	}
}

// Tick decrements frame counter, returns true if toast should dismiss
func (t *ToastState) Tick() bool {
	if !t.Visible {
		return false
	}
	if t.FramesLeft < 0 {
		return false // Persistent
	}
	t.FramesLeft--
	if t.FramesLeft <= 0 {
		t.Visible = false
		return true
	}
	return false
}

// Dismiss hides the toast
func (t *ToastState) Dismiss() {
	t.Visible = false
}

// Show displays a new toast
func (t *ToastState) Show(opts ToastOpts, frames int) {
	t.Opts = opts
	t.FramesLeft = frames
	t.Visible = true
}

