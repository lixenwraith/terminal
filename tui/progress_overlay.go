package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ProgressType specifies progress indicator variant
type ProgressType uint8

const (
	ProgressSpinner       ProgressType = iota // Animated spinner
	ProgressDeterminate                       // Bar with percentage
	ProgressIndeterminate                     // Marquee animation
	ProgressPulse                             // Pulsing bar
	ProgressDots                              // Animated dots
)

// ProgressStyle defines visual appearance
type ProgressStyle uint8

const (
	ProgressStyleMinimal ProgressStyle = iota // Text only
	ProgressStyleBox                          // Single border
	ProgressStyleDouble                       // Double border
	ProgressStyleRounded                      // Rounded border
	ProgressStyleShadow                       // Box with shadow
	ProgressStyleNeon                         // Bright colors
	ProgressStyleRetro                        // Block characters
)

// SpinnerStyle defines spinner animation type
type SpinnerStyle uint8

const (
	SpinnerBraille SpinnerStyle = iota // ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏
	SpinnerDots                        // ⣾⣽⣻⢿⡿⣟⣯⣷
	SpinnerLine                        // |/-\
	SpinnerBlock                       // ▖▘▝▗
	SpinnerCircle                      // ◐◓◑◒
	SpinnerArc                         // ◜◠◝◞◡◟
	SpinnerBounce                      // ⠁⠂⠄⠂
	SpinnerGrow                        // ▁▃▄▅▆▇█▇▆▅▄▃
)

// Spinner frame sets
var spinnerSets = map[SpinnerStyle][]rune{
	SpinnerBraille: {'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'},
	SpinnerDots:    {'⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'},
	SpinnerLine:    {'|', '/', '-', '\\'},
	SpinnerBlock:   {'▖', '▘', '▝', '▗'},
	SpinnerCircle:  {'◐', '◓', '◑', '◒'},
	SpinnerArc:     {'◜', '◠', '◝', '◞', '◡', '◟'},
	SpinnerBounce:  {'⠁', '⠂', '⠄', '⠂'},
	SpinnerGrow:    {'▁', '▃', '▄', '▅', '▆', '▇', '█', '▇', '▆', '▅', '▄', '▃'},
}

// BarStyle defines progress bar appearance
type BarStyle uint8

const (
	BarStyleBlock   BarStyle = iota // █░
	BarStyleShade                   // ▓▒░
	BarStyleArrow                   // =>-
	BarStyleDot                     // ●○
	BarStyleBracket                 // [###   ]
	BarStylePipe                    // |===  |
	BarStyleThin                    // ━╺
	BarStyleThick                   // ▰▱
)

// Bar character sets: [filled, partial, empty]
var barCharSets = map[BarStyle][3]rune{
	BarStyleBlock:   {'█', '▌', '░'},
	BarStyleShade:   {'▓', '▒', '░'},
	BarStyleArrow:   {'=', '>', '-'},
	BarStyleDot:     {'●', '◐', '○'},
	BarStyleBracket: {'#', '#', ' '},
	BarStylePipe:    {'=', '=', ' '},
	BarStyleThin:    {'━', '╸', '╺'},
	BarStyleThick:   {'▰', '▰', '▱'},
}

// ProgressOverlayOpts configures progress overlay
type ProgressOverlayOpts struct {
	Title        string
	Message      string
	Type         ProgressType
	Style        ProgressStyle
	SpinnerStyle SpinnerStyle
	BarStyle     BarStyle
	Progress     float64 // 0.0-1.0 for determinate
	Frame        int     // Animation frame counter
	ShowPercent  bool    // Show percentage text
	ShowETA      string  // Optional ETA string
	Width        int     // Overlay width, 0 = auto
	Cancelable   bool    // Show cancel hint
	CancelKey    string  // e.g., "Esc"
	Fg           color.RGB
	Bg           color.RGB
	BarFg        color.RGB
	BarBg        color.RGB
	AccentFg     color.RGB // Spinner/highlight color
}

// DefaultProgressOpts returns sensible defaults
func DefaultProgressOpts(title, message string, ptype ProgressType) ProgressOverlayOpts {
	return ProgressOverlayOpts{
		Title:        title,
		Message:      message,
		Type:         ptype,
		Style:        ProgressStyleBox,
		SpinnerStyle: SpinnerBraille,
		BarStyle:     BarStyleBlock,
		ShowPercent:  true,
		Width:        40,
		Fg:           color.RGB{R: 220, G: 220, B: 220},
		Bg:           color.RGB{R: 30, G: 30, B: 40},
		BarFg:        color.RGB{R: 80, G: 160, B: 255},
		BarBg:        color.RGB{R: 50, G: 50, B: 60},
		AccentFg:     color.RGB{R: 100, G: 200, B: 255},
	}
}

// ProgressOverlay renders centered progress overlay
func (r Region) ProgressOverlay(opts ProgressOverlayOpts) Region {
	if r.W < 10 || r.H < 5 {
		return Region{}
	}

	// Calculate overlay dimensions
	overlayW := opts.Width
	if overlayW == 0 {
		overlayW = 40
	}
	if overlayW > r.W-4 {
		overlayW = r.W - 4
	}

	// Height: title + message + progress + cancel hint
	overlayH := 3 // border top/bottom + 1 content
	if opts.Title != "" {
		overlayH++ // Title takes space on border
	}
	if opts.Message != "" {
		overlayH++
	}
	if opts.Type != ProgressSpinner && opts.Type != ProgressDots {
		overlayH++ // Progress bar row
	}
	if opts.Cancelable {
		overlayH++
	}
	if overlayH > r.H-2 {
		overlayH = r.H - 2
	}

	// Center overlay
	overlay := Center(r, overlayW, overlayH)

	// Determine border type
	var borderLine LineType
	switch opts.Style {
	case ProgressStyleMinimal:
		borderLine = LineNone
	case ProgressStyleBox:
		borderLine = LineSingle
	case ProgressStyleDouble:
		borderLine = LineDouble
	case ProgressStyleRounded:
		borderLine = LineRounded
	case ProgressStyleShadow:
		// Draw shadow first
		shadow := r.Sub(overlay.X-r.X+1, overlay.Y-r.Y+1, overlayW, overlayH)
		shadow.Fill(color.RGB{R: 10, G: 10, B: 15})
		borderLine = LineSingle
	case ProgressStyleNeon:
		borderLine = LineDouble
		opts.AccentFg = color.RGB{R: 0, G: 255, B: 200}
		opts.BarFg = color.RGB{R: 255, G: 0, B: 255}
	case ProgressStyleRetro:
		borderLine = LineHeavy
		opts.BarStyle = BarStyleBlock
	}

	// Draw frame
	overlay.BoxFilled(borderLine, opts.Fg, opts.Bg)

	// Title on border
	if opts.Title != "" {
		title := " " + opts.Title + " "
		titleLen := RuneLen(title)
		if titleLen > overlayW-4 {
			title = Truncate(title, overlayW-4)
			titleLen = RuneLen(title)
		}
		titleX := (overlayW - titleLen) / 2
		for i, ch := range title {
			overlay.Cell(titleX+i, 0, ch, opts.AccentFg, opts.Bg, terminal.AttrBold)
		}
	}

	// Content area
	content := overlay.Inset(1)
	y := 0

	// Spinner/dots for spinner types - inline with message
	if opts.Type == ProgressSpinner || opts.Type == ProgressDots {
		spinnerChar := r.getSpinnerChar(opts)
		if content.W > 2 {
			content.Cell(0, y, spinnerChar, opts.AccentFg, opts.Bg, terminal.AttrBold)
		}

		// Message after spinner
		if opts.Message != "" {
			msg := opts.Message
			availW := content.W - 2
			if RuneLen(msg) > availW {
				msg = Truncate(msg, availW)
			}
			content.Text(2, y, msg, opts.Fg, opts.Bg, terminal.AttrNone)
		}
		y++
	} else {
		// Message on its own line
		if opts.Message != "" {
			msg := opts.Message
			if RuneLen(msg) > content.W {
				msg = Truncate(msg, content.W)
			}
			content.TextCenter(y, msg, opts.Fg, opts.Bg, terminal.AttrNone)
			y++
		}

		// Progress bar
		if y < content.H {
			r.renderProgressBar(content.Sub(0, y, content.W, 1), opts)
			y++
		}
	}

	// Cancel hint
	if opts.Cancelable && y < content.H {
		hint := opts.CancelKey + " to cancel"
		if opts.CancelKey == "" {
			hint = "Esc to cancel"
		}
		content.TextCenter(y, hint, color.RGB{R: 120, G: 120, B: 130}, opts.Bg, terminal.AttrDim)
	}

	return overlay
}

func (r Region) getSpinnerChar(opts ProgressOverlayOpts) rune {
	frames := spinnerSets[opts.SpinnerStyle]
	if len(frames) == 0 {
		frames = spinnerSets[SpinnerBraille]
	}
	idx := opts.Frame % len(frames)
	if idx < 0 {
		idx = -idx
	}
	return frames[idx]
}

func (r Region) renderProgressBar(bar Region, opts ProgressOverlayOpts) {
	if bar.W < 3 || bar.H < 1 {
		return
	}

	barW := bar.W
	labelW := 0

	// Reserve space for percentage
	if opts.ShowPercent {
		labelW = 5 // " 100%"
		barW -= labelW
	}

	// Reserve space for ETA
	if opts.ShowETA != "" {
		etaW := RuneLen(opts.ShowETA) + 1
		barW -= etaW
	}

	if barW < 3 {
		barW = 3
	}

	chars := barCharSets[opts.BarStyle]

	switch opts.Type {
	case ProgressDeterminate:
		pct := opts.Progress
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}

		filled := int(float64(barW) * pct)
		remainder := float64(barW)*pct - float64(filled)

		for x := 0; x < barW; x++ {
			var ch rune
			var fg color.RGB
			if x < filled {
				ch = chars[0]
				fg = opts.BarFg
			} else if x == filled && remainder >= 0.5 {
				ch = chars[1]
				fg = opts.BarFg
			} else {
				ch = chars[2]
				fg = opts.BarBg
			}
			bar.Cell(x, 0, ch, fg, opts.Bg, terminal.AttrNone)
		}

		// Percentage
		if opts.ShowPercent {
			pctStr := formatPercent(int(pct * 100))
			bar.Text(barW+1, 0, pctStr, opts.Fg, opts.Bg, terminal.AttrNone)
		}

	case ProgressIndeterminate:
		// Marquee effect
		pos := opts.Frame % (barW * 2)
		if pos >= barW {
			pos = barW*2 - pos - 1
		}
		markerW := barW / 4
		if markerW < 2 {
			markerW = 2
		}

		for x := 0; x < barW; x++ {
			var ch rune
			var fg color.RGB
			if x >= pos && x < pos+markerW {
				ch = chars[0]
				fg = opts.BarFg
			} else {
				ch = chars[2]
				fg = opts.BarBg
			}
			bar.Cell(x, 0, ch, fg, opts.Bg, terminal.AttrNone)
		}

	case ProgressPulse:
		// Pulsing intensity based on frame
		pulsePhase := opts.Frame % 20
		intensity := float64(pulsePhase) / 20.0
		if pulsePhase > 10 {
			intensity = 1.0 - float64(pulsePhase-10)/10.0
		}

		pct := opts.Progress
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}

		filled := int(float64(barW) * pct)

		for x := 0; x < barW; x++ {
			var ch rune
			var fg color.RGB
			if x < filled {
				ch = chars[0]
				// Pulse the color
				fg = color.RGB{
					R: uint8(float64(opts.BarFg.R) * (0.5 + intensity*0.5)),
					G: uint8(float64(opts.BarFg.G) * (0.5 + intensity*0.5)),
					B: uint8(float64(opts.BarFg.B) * (0.5 + intensity*0.5)),
				}
			} else {
				ch = chars[2]
				fg = opts.BarBg
			}
			bar.Cell(x, 0, ch, fg, opts.Bg, terminal.AttrNone)
		}

		if opts.ShowPercent {
			pctStr := formatPercent(int(pct * 100))
			bar.Text(barW+1, 0, pctStr, opts.Fg, opts.Bg, terminal.AttrNone)
		}
	}

	// ETA
	if opts.ShowETA != "" {
		etaX := bar.W - RuneLen(opts.ShowETA)
		bar.Text(etaX, 0, opts.ShowETA, color.RGB{R: 150, G: 150, B: 160}, opts.Bg, terminal.AttrDim)
	}
}

func formatPercent(pct int) string {
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	if pct == 100 {
		return "100%"
	}
	if pct >= 10 {
		return " " + string(rune('0'+pct/10)) + string(rune('0'+pct%10)) + "%"
	}
	return "  " + string(rune('0'+pct)) + "%"
}

// ProgressState manages progress overlay state
type ProgressState struct {
	Visible  bool
	Opts     ProgressOverlayOpts
	Frame    int
	Progress float64
}

// NewProgressState creates progress overlay state
func NewProgressState(opts ProgressOverlayOpts) *ProgressState {
	return &ProgressState{
		Visible:  true,
		Opts:     opts,
		Progress: opts.Progress,
	}
}

// Tick advances animation frame
func (p *ProgressState) Tick() {
	p.Frame++
	p.Opts.Frame = p.Frame
}

// SetProgress updates progress value (0.0-1.0)
func (p *ProgressState) SetProgress(pct float64) {
	p.Progress = pct
	p.Opts.Progress = pct
}

// SetMessage updates message text
func (p *ProgressState) SetMessage(msg string) {
	p.Opts.Message = msg
}

// SetETA updates ETA string
func (p *ProgressState) SetETA(eta string) {
	p.Opts.ShowETA = eta
}

// Complete marks progress as done
func (p *ProgressState) Complete() {
	p.Progress = 1.0
	p.Opts.Progress = 1.0
}

// Dismiss hides the progress overlay
func (p *ProgressState) Dismiss() {
	p.Visible = false
}

// Show displays the progress overlay
func (p *ProgressState) Show() {
	p.Visible = true
}

