package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// Region represents a rectangular area within a cell buffer
// All coordinates are relative to the region's origin
type Region struct {
	Cells  []terminal.Cell
	TotalW int // Total width of the underlying cell buffer
	X, Y   int // Absolute position in cell buffer
	W, H   int // Region dimensions
}

// NewRegion creates a region referencing a cell slice with bounds
func NewRegion(cells []terminal.Cell, totalW, x, y, w, h int) Region {
	return Region{
		Cells:  cells,
		TotalW: totalW,
		X:      x,
		Y:      y,
		W:      w,
		H:      h,
	}
}

// Sub returns a nested region with coordinates relative to parent, result is clipped to parent bounds
func (r Region) Sub(x, y, w, h int) Region {
	// Clip to parent bounds
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > r.W {
		w = r.W - x
	}
	if y+h > r.H {
		h = r.H - y
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}

	return Region{
		Cells:  r.Cells,
		TotalW: r.TotalW,
		X:      r.X + x,
		Y:      r.Y + y,
		W:      w,
		H:      h,
	}
}

// Inset returns a region shrunk by n cells on all sides
func (r Region) Inset(n int) Region {
	return r.Sub(n, n, r.W-2*n, r.H-2*n)
}

// Cell sets a single cell with bounds checking
func (r Region) Cell(x, y int, ch rune, fg, bg color.RGB, attr terminal.Attr) {
	if x < 0 || x >= r.W || y < 0 || y >= r.H {
		return
	}
	absX := r.X + x
	absY := r.Y + y

	// Bounds check against the physical buffer dimensions
	if uint(absX) >= uint(r.TotalW) {
		return
	}

	idx := absY*r.TotalW + absX
	// Single bounds check for the backing slice
	if uint(idx) < uint(len(r.Cells)) {
		r.Cells[idx] = terminal.Cell{Rune: ch, Fg: fg, Bg: bg, Attrs: attr}
	}
}

// Fill fills entire region with background color
func (r Region) Fill(bg color.RGB) {
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			r.Cell(x, y, ' ', color.RGB{}, bg, terminal.AttrNone)
		}
	}
}

// Clear fills region with spaces and zero colors
func (r Region) Clear() {
	r.Fill(color.RGB{})
}

// Width returns region width
func (r Region) Width() int {
	return r.W
}

// Height returns region height
func (r Region) Height() int {
	return r.H
}

// Bounds returns absolute position and dimensions
func (r Region) Bounds() (x, y, w, h int) {
	return r.X, r.Y, r.W, r.H
}

