package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ScrollBarOpts configures scrollbar rendering
type ScrollBarOpts struct {
	ThumbFg  color.RGB
	TrackFg  color.RGB
	Bg       color.RGB // Zero inherits the existing cell background
	Thumb    rune      // 0 = '█'
	Track    rune      // 0 = '░'
	HideIdle bool      // Draw nothing when content fits the viewport
}

// ScrollBar draws a vertical scrollbar track with thumb in a single color
func (r Region) ScrollBar(x int, offset, visible, total int, fg color.RGB) {
	r.ScrollBarStyled(x, offset, visible, total, ScrollBarOpts{ThumbFg: fg, TrackFg: fg})
}

// ScrollBarStyled draws a vertical scrollbar at column x with explicit styling
func (r Region) ScrollBarStyled(x int, offset, visible, total int, opts ScrollBarOpts) {
	if x < 0 || x >= r.W || r.H < 1 {
		return
	}

	thumbCh, trackCh := opts.Thumb, opts.Track
	if thumbCh == 0 {
		thumbCh = '█'
	}
	if trackCh == 0 {
		trackCh = '░'
	}

	trackH := r.H
	thumbY, thumbH := ScrollThumb(trackH, offset, visible, total)
	if thumbH == 0 {
		if opts.HideIdle {
			return
		}
		for y := range trackH {
			r.Cell(x, y, '│', opts.TrackFg, opts.Bg, terminal.AttrDim)
		}
		return
	}

	for y := range trackH {
		if y >= thumbY && y < thumbY+thumbH {
			r.Cell(x, y, thumbCh, opts.ThumbFg, opts.Bg, terminal.AttrNone)
		} else {
			r.Cell(x, y, trackCh, opts.TrackFg, opts.Bg, terminal.AttrDim)
		}
	}
}

// ScrollThumb is the thumb a scrollbar trackH rows tall draws for offset: its
// first row and its height, 0 when content fits or the track is under 3 rows
func ScrollThumb(trackH, offset, visible, total int) (y, h int) {
	if total <= visible || trackH < 3 {
		return 0, 0
	}
	h = min(max((visible*trackH)/total, 1), trackH)
	y = (offset * (trackH - h)) / (total - visible)
	return min(max(y, 0), trackH-h), h
}

// ScrollDrag holds a scrollbar's thumb under the pointer, rows counted from the
// track's top: a press on the thumb takes hold where it lands, a press on the
// track moves the thumb's middle there first, and each drag keeps the row held
// under the pointer. Each call returns the offset to show.
type ScrollDrag struct {
	Active bool // The thumb is held
	Grab   int  // The thumb's row the pointer holds
}

// Press takes hold of the thumb at track row y; off the track it holds nothing
func (d *ScrollDrag) Press(y, trackH, offset, visible, total int) int {
	top, h := ScrollThumb(trackH, offset, visible, total)
	d.Active = h > 0 && y >= 0 && y < trackH
	if !d.Active {
		return offset
	}
	if y < top || y >= top+h {
		offset = scrollOffsetAt(y-h/2, trackH, h, visible, total)
		top, _ = ScrollThumb(trackH, offset, visible, total)
	}
	d.Grab = min(max(y-top, 0), h-1)
	return offset
}

// Drag moves the held row to track row y, clamped at the track's ends
func (d *ScrollDrag) Drag(y, trackH, offset, visible, total int) int {
	_, h := ScrollThumb(trackH, offset, visible, total)
	if !d.Active || h == 0 {
		return offset
	}
	return scrollOffsetAt(y-d.Grab, trackH, h, visible, total)
}

// Release lets go of the thumb
func (d *ScrollDrag) Release() { *d = ScrollDrag{} }

// scrollOffsetAt is the offset whose thumb, h rows tall, starts nearest row top
func scrollOffsetAt(top, trackH, h, visible, total int) int {
	span, maxScroll := trackH-h, total-visible
	top = min(max(top, 0), span)
	o := (top*maxScroll + span - 1) / span // The first offset whose thumb starts at or past top
	if o > 0 {
		at, _ := ScrollThumb(trackH, o, visible, total)
		before, _ := ScrollThumb(trackH, o-1, visible, total)
		if at-top > top-before {
			return o - 1
		}
	}
	return o
}

// ScrollIndicator draws compact indicator text (Top/Bot/XX%)
func (r Region) ScrollIndicator(y int, offset, visible, total int, fg color.RGB) {
	if y < 0 || y >= r.H {
		return
	}

	var text string
	if total <= visible || offset <= 0 {
		text = "Top"
	} else if offset+visible >= total {
		text = "Bot"
	} else {
		pct := ScrollPercent(offset, visible, total)
		if pct >= 100 {
			text = "99%"
		} else if pct >= 10 {
			text = string(rune('0'+pct/10)) + string(rune('0'+pct%10)) + "%"
		} else {
			text = " " + string(rune('0'+pct)) + "%"
		}
	}

	r.TextRight(y, text, fg, color.RGB{}, terminal.AttrDim)
}
