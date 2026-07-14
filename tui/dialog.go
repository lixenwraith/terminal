package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ConfirmResult represents dialog outcome
type ConfirmResult uint8

const (
	ConfirmPending ConfirmResult = iota
	ConfirmYes
	ConfirmNo
	ConfirmCancel
)

// ConfirmState holds confirmation dialog state
type ConfirmState struct {
	FocusYes bool // true = Yes focused, false = No focused
	Result   ConfirmResult
}

// NewConfirmState creates dialog state with default selection
func NewConfirmState(defaultYes bool) *ConfirmState {
	return &ConfirmState{
		FocusYes: defaultYes,
		Result:   ConfirmPending,
	}
}

// Toggle switches focus between Yes and No
func (c *ConfirmState) Toggle() {
	c.FocusYes = !c.FocusYes
}

// Confirm selects currently focused button
func (c *ConfirmState) Confirm() {
	if c.FocusYes {
		c.Result = ConfirmYes
	} else {
		c.Result = ConfirmNo
	}
}

// SelectYes directly selects Yes
func (c *ConfirmState) SelectYes() {
	c.Result = ConfirmYes
}

// SelectNo directly selects No
func (c *ConfirmState) SelectNo() {
	c.Result = ConfirmNo
}

// Cancel cancels the dialog
func (c *ConfirmState) Cancel() {
	c.Result = ConfirmCancel
}

// HandleKey processes input, returns true if dialog should close
func (c *ConfirmState) HandleKey(key terminal.Key, r rune) bool {
	switch key {
	case terminal.KeyLeft, terminal.KeyRight, terminal.KeyTab:
		c.Toggle()
		return false
	case terminal.KeyEnter:
		c.Confirm()
		return true
	case terminal.KeyEscape:
		c.Cancel()
		return true
	case terminal.KeyRune:
		switch r {
		case 'y', 'Y':
			c.SelectYes()
			return true
		case 'n', 'N':
			c.SelectNo()
			return true
		case 'h':
			c.FocusYes = true
			return false
		case 'l':
			c.FocusYes = false
			return false
		}
	}
	return false
}

// ConfirmOpts configures confirmation dialog
type ConfirmOpts struct {
	Title       string
	Message     string
	YesLabel    string // Default "Yes"
	NoLabel     string // Default "No"
	Destructive bool   // Style Yes as warning
	Style       ConfirmStyle
}

// ConfirmStyle defines dialog colors
type ConfirmStyle struct {
	BorderFg      color.RGB
	TitleFg       color.RGB
	MessageFg     color.RGB
	Bg            color.RGB
	ButtonFg      color.RGB
	ButtonBg      color.RGB
	ButtonFocusFg color.RGB
	ButtonFocusBg color.RGB
	DestructiveFg color.RGB
	DestructiveBg color.RGB
}

// DefaultConfirmStyle returns default dialog colors
func DefaultConfirmStyle() ConfirmStyle {
	return ConfirmStyle{
		BorderFg:      color.RGB{R: 100, G: 100, B: 120},
		TitleFg:       color.RGB{R: 255, G: 255, B: 255},
		MessageFg:     color.RGB{R: 200, G: 200, B: 200},
		Bg:            color.RGB{R: 30, G: 30, B: 40},
		ButtonFg:      color.RGB{R: 180, G: 180, B: 180},
		ButtonBg:      color.RGB{R: 50, G: 50, B: 60},
		ButtonFocusFg: color.RGB{R: 255, G: 255, B: 255},
		ButtonFocusBg: color.RGB{R: 60, G: 80, B: 120},
		DestructiveFg: color.RGB{R: 255, G: 255, B: 255},
		DestructiveBg: color.RGB{R: 180, G: 60, B: 60},
	}
}

// ConfirmDialog renders confirmation dialog centered in region
// Returns content region for additional content if needed
func (r Region) ConfirmDialog(state *ConfirmState, opts ConfirmOpts) Region {
	style := opts.Style
	if style == (ConfirmStyle{}) {
		style = DefaultConfirmStyle()
	}

	if opts.YesLabel == "" {
		opts.YesLabel = "Yes"
	}
	if opts.NoLabel == "" {
		opts.NoLabel = "No"
	}

	// Calculate dialog size
	msgLines := WrapText(opts.Message, r.W-8)
	if len(msgLines) == 0 {
		msgLines = []string{""}
	}

	dialogW := 40
	msgMaxW := 0
	for _, line := range msgLines {
		if w := RuneLen(line); w > msgMaxW {
			msgMaxW = w
		}
	}
	if msgMaxW+6 > dialogW {
		dialogW = msgMaxW + 6
	}
	if dialogW > r.W-4 {
		dialogW = r.W - 4
	}
	if dialogW < 20 {
		dialogW = 20
	}

	dialogH := 3 + len(msgLines) + 3 // border + message + spacing + buttons + border
	if dialogH > r.H-2 {
		dialogH = r.H - 2
	}

	// Center dialog
	dialog := Center(r, dialogW, dialogH)

	// Draw modal frame
	content := dialog.Modal(ModalOpts{
		Title:    opts.Title,
		Border:   LineDouble,
		BorderFg: style.BorderFg,
		TitleFg:  style.TitleFg,
		Bg:       style.Bg,
	})

	// Message
	y := 0
	for _, line := range msgLines {
		if y >= content.H-2 {
			break
		}
		content.TextCenter(y, line, style.MessageFg, style.Bg, terminal.AttrNone)
		y++
	}

	// Buttons row
	buttonY := content.H - 1
	if buttonY < y+1 {
		buttonY = y + 1
	}

	yesLabel := " " + opts.YesLabel + " "
	noLabel := " " + opts.NoLabel + " "

	yesW := RuneLen(yesLabel)
	noW := RuneLen(noLabel)
	buttonGap := 4
	totalButtonW := yesW + buttonGap + noW

	buttonX := (content.W - totalButtonW) / 2
	if buttonX < 0 {
		buttonX = 0
	}

	// Yes button
	yesFg := style.ButtonFg
	yesBg := style.ButtonBg
	if state.FocusYes {
		if opts.Destructive {
			yesFg = style.DestructiveFg
			yesBg = style.DestructiveBg
		} else {
			yesFg = style.ButtonFocusFg
			yesBg = style.ButtonFocusBg
		}
	}
	for i, ch := range yesLabel {
		if buttonX+i < content.W {
			content.Cell(buttonX+i, buttonY, ch, yesFg, yesBg, terminal.AttrNone)
		}
	}

	// No button
	noX := buttonX + yesW + buttonGap
	noFg := style.ButtonFg
	noBg := style.ButtonBg
	if !state.FocusYes {
		noFg = style.ButtonFocusFg
		noBg = style.ButtonFocusBg
	}
	for i, ch := range noLabel {
		if noX+i < content.W {
			content.Cell(noX+i, buttonY, ch, noFg, noBg, terminal.AttrNone)
		}
	}

	return content.Sub(0, 0, content.W, buttonY-1)
}

// AlertOpts configures single-button alert dialog
type AlertOpts struct {
	Title   string
	Message string
	Button  string // Default "OK"
	Style   ConfirmStyle
}

// AlertDialog renders single-button alert, returns true when dismissed
func (r Region) AlertDialog(opts AlertOpts) Region {
	style := opts.Style
	if style == (ConfirmStyle{}) {
		style = DefaultConfirmStyle()
	}

	if opts.Button == "" {
		opts.Button = "OK"
	}

	msgLines := WrapText(opts.Message, r.W-8)
	if len(msgLines) == 0 {
		msgLines = []string{""}
	}

	dialogW := 36
	msgMaxW := 0
	for _, line := range msgLines {
		if w := RuneLen(line); w > msgMaxW {
			msgMaxW = w
		}
	}
	if msgMaxW+6 > dialogW {
		dialogW = msgMaxW + 6
	}
	if dialogW > r.W-4 {
		dialogW = r.W - 4
	}

	dialogH := 3 + len(msgLines) + 3
	if dialogH > r.H-2 {
		dialogH = r.H - 2
	}

	dialog := Center(r, dialogW, dialogH)

	content := dialog.Modal(ModalOpts{
		Title:    opts.Title,
		Border:   LineDouble,
		BorderFg: style.BorderFg,
		TitleFg:  style.TitleFg,
		Bg:       style.Bg,
	})

	// Message
	y := 0
	for _, line := range msgLines {
		if y >= content.H-2 {
			break
		}
		content.TextCenter(y, line, style.MessageFg, style.Bg, terminal.AttrNone)
		y++
	}

	// Button
	buttonY := content.H - 1
	buttonLabel := " " + opts.Button + " "
	buttonW := RuneLen(buttonLabel)
	buttonX := (content.W - buttonW) / 2

	for i, ch := range buttonLabel {
		if buttonX+i < content.W {
			content.Cell(buttonX+i, buttonY, ch, style.ButtonFocusFg, style.ButtonFocusBg, terminal.AttrNone)
		}
	}

	return content.Sub(0, 0, content.W, buttonY-1)
}

