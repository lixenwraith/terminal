// Package android is the Go half of an Android view showing a Terminal's cells:
// the grid its pixels hold, its keys, touches and text as Events, the frames its
// drawer takes and the texture it paints them from. An app's binding calls a View
// from the view's own threads; nothing here imports one.
package android

import (
	"cmp"
	"sync"
	"sync/atomic"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/glyph"
)

// eventQueue holds what the view posts while its program is busy; a post never
// waits, since the poster is Android's UI thread
const eventQueue = 256

// DefaultCell is a cell's height in dp until a pinch zooms it, in whole pixels at
// the face's aspect: 16x32 on a 420 dpi phone
const DefaultCell = 12

// stripRows is the rows a drawer's strip above the keyboard takes: the grid's last
// row drawn again there at twice its height
const stripRows = 2

// Config is what a view opens with
type Config struct {
	Font    *glyph.Font
	Raster  bool       // the drawer shows the raster's pixels, not cells of the texture
	Density float32    // the screen's pixels to a dp
	Cell    float64    // a kept zoom, a cell's height in dp, 0 for DefaultCell
	Zoom    glyph.Zoom // the range a pinch keeps the cell to
	BarRows int        // the rows at the grid's foot a finger presses with the pointer off, 0 for none
}

// View is an Android view as a terminal.Host. Frames wait in the embedded
// terminal.Frames for the drawer to take them; the grid is the cells the view's
// pixels hold, and the drawer stretches it over them, so a touch maps in
// proportion. The UI thread calls what sizes the view and reads its input, the
// drawer's thread AtlasChanged and Take, the program the rest.
type View struct {
	*terminal.Frames
	font    *glyph.Font
	raster  bool
	density float64
	keeps   glyph.Zoom
	barRows int

	events  chan terminal.Event
	keys    terminal.HostKeys
	pointer atomic.Bool
	covered atomic.Int32 // the pixels at the view's foot the keyboard covers, 0 while it is down
	finger  struct {     // the cell a pressed finger last reported; the UI thread's alone
		down bool
		x, y int
	}

	mu           sync.Mutex
	cols, rows   int            // the grid the view's pixels hold
	drawn        [2]int         // the grid of the newest frame the program flushed
	w, h         int            // those pixels, 0 until the first Resize
	cell         float64        // the zoom: a cell's height in dp, 0 for DefaultCell
	cellW, cellH int            // the pixels a cell takes at least, sized by the density and the zoom
	strike       *glyph.Atlas   // the strike that draws such a cell
	tex          *glyph.Texture // what frames are encoded for, which AtlasChanged brings to strike
}

var _ terminal.Host = (*View)(nil)

// NewView opens a view with no pixels yet: its first Resize gives it its grid,
// before its program reads Size or a finger lands
func NewView(c Config) *View {
	v := &View{font: c.Font, raster: c.Raster, density: float64(c.Density), keeps: c.Zoom, barRows: c.BarRows,
		events: make(chan terminal.Event, eventQueue), cell: c.Cell}
	v.keys.Post = v.Post
	v.setCell()
	v.tex = glyph.NewTexture(v.strike)
	v.Frames = terminal.NewFrames(v.encoder())
	return v
}

// setCell sizes a cell by the density and the zoom, which names its strike; mu held
func (v *View) setCell() {
	v.strike, v.cellW, v.cellH = v.font.Cell(v.density, cmp.Or(v.cell, DefaultCell))
}

// encoder writes frames as the drawer holding tex reads them: cells naming its
// slots, or the pixels of its strike's raster
func (v *View) encoder() terminal.FrameEncoder {
	if v.raster {
		return glyph.NewRaster(v.tex.Atlas()).Frame
	}
	return v.tex.Frame
}

// Atlas is the texture of the strike frames are encoded for, as glyph.NewTexture
// lays it out; a drawer of the raster's pixels reads the cell's size from it
func (v *View) Atlas() []byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.tex.Bytes()
}

// Flush keeps a frame for the drawer and notes its grid, which AtlasChanged waits on
func (v *View) Flush(cells []terminal.Cell, width, height int) {
	v.Frames.Flush(cells, width, height)
	v.mu.Lock()
	v.drawn = [2]int{width, height}
	v.mu.Unlock()
}

// AtlasChanged is Atlas once after a zoom crossed to another strike and the program
// drew the zoomed grid, nil otherwise. The drawer's thread calls it before each Take
// and paints from what it returns: the frames switch to that strike in the same
// call, the next one every cell, so none is encoded for a texture the drawer does
// not hold, and the drawer never repaints the grid the zoom left in the new strike.
func (v *View) AtlasChanged() []byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.strike == v.tex.Atlas() || v.drawn != [2]int{v.cols, v.rows} {
		return nil
	}
	v.tex = glyph.NewTexture(v.strike)
	v.SetEncoder(v.encoder())
	return v.tex.Bytes()
}

// Zoom scales the cell by a pinch's scale, a step of the range and never under the
// font's smallest strike, and refits the grid to the view's pixels
func (v *View) Zoom(scale float64) {
	v.mu.Lock()
	v.cell = v.keeps.Scale(v.font, v.density, cmp.Or(v.cell, DefaultCell), scale)
	v.setCell()
	w, h := v.w, v.h
	v.mu.Unlock()
	if w > 0 {
		v.Resize(w, h)
	}
}

// Cell is the zoom the player left the view at, a cell's height in dp, 0 for
// DefaultCell
func (v *View) Cell() float64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cell
}

// Aspect is the width over the height of the cells the view shows: the face's,
// whichever strike draws them
func (v *View) Aspect() float64 {
	strikes := v.font.Strikes()
	return strikes[len(strikes)-1].Aspect()
}

// Resize follows the view's pixels: the grid is the whole cells they hold, at
// least one each way. It reports whether the grid changed, and posts the change as
// a resize unless the grid is the view's first, the one its program opens on.
func (v *View) Resize(width, height int) bool {
	v.mu.Lock()
	cols, rows := max(1, width/v.cellW), max(1, height/v.cellH)
	changed, first := cols != v.cols || rows != v.rows, v.w == 0
	v.cols, v.rows, v.w, v.h = cols, rows, max(1, width), max(1, height)
	v.mu.Unlock()
	if changed && !first {
		v.Post(terminal.Event{Type: terminal.EventResize, Width: cols, Height: rows})
	}
	return changed
}

func (v *View) Size() (cols, rows int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cols, v.rows
}

// SetCovered is the pixels at the view's foot the system keyboard covers, 0 while
// it is down
func (v *View) SetCovered(px int) { v.covered.Store(int32(max(px, 0))) }

// Covered is the grid's rows the keyboard hides with the strip above it, the drawer
// stretching the grid over the view's pixels
func (v *View) Covered() int {
	px := int(v.covered.Load())
	v.mu.Lock()
	defer v.mu.Unlock()
	if px == 0 || v.h == 0 {
		return 0
	}
	return min((px*v.rows+v.h-1)/v.h+stripRows, v.rows)
}

func (v *View) Events() <-chan terminal.Event { return v.events }

// Post queues an event for the program; with the queue full the event is lost,
// which a UI thread that must not wait on the program prefers
func (v *View) Post(ev terminal.Event) {
	select {
	case v.events <- ev:
	default:
	}
}

// SetPointerMode drops touches while the program takes no pointer; a finger has no
// motion of its own to report
func (v *View) SetPointerMode(enabled, _ bool) { v.pointer.Store(enabled) }

// SetKeyReport asks for each key's release, which a hardware key's up gives; the
// system keyboard's text and the keys the view makes let go as they press
func (v *View) SetKeyReport(report terminal.KeyReport, allow terminal.KeySource) error {
	v.keys.Ask(report, allow, true)
	return nil
}
