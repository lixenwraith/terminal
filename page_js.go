//go:build js && wasm

package terminal

import (
	"errors"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"
)

// pageQueue matches the tty reader's; a post never waits, since the poster is the
// worker's event loop, which a blocked callback would hold
const pageQueue = 256

// fitWait bounds OpenPage's wait for the page to fit its grid to the zoom: a round
// trip to the page's thread, so only a page that never answers runs out of it
const fitWait = time.Second

// jsPage is the bridge, the one global the page's worker defines before the program
// runs: grid, kept, features and hidden as the page has them, frame and post, which
// hand the page a frame and a message, and event, which Go sets to take the page's
const jsPage = "terminalPage"

// PageConfig is what a program tells the page it opens on
type PageConfig struct {
	// BarRows is the rows at the grid's foot a finger still presses with the
	// pointer off, and whose three taps raise or drop the keyboard; 0 for none
	BarRows int
	// Cell is a cell's height in CSS pixels the page opens at, 0 for its own;
	// CellLeast, CellMost and CellStep the range and step a zoom keeps to
	Cell, CellLeast, CellMost, CellStep float64
	// Font is a web font (woff2, ttf) the page draws glyphs with, nil for its own
	Font []byte
}

// Page is a Host over a web page: the program runs in a Worker, and the page draws
// the frames handed to it, one at a time and the newest once it has drawn the last,
// and reports its keys, its pointer and its size as the events a terminal would.
// Flush and Sync may be called from any goroutine; the page's messages arrive on
// the worker's event loop.
type Page struct {
	*Frames
	events  chan Event
	grid    atomic.Uint64 // cols<<32 | rows
	keys    HostKeys
	barRows int           // the bottom rows a finger still presses with the pointer off
	pointer atomic.Bool   // the program takes pointer events
	covered atomic.Int32  // the grid's rows at the foot a keyboard and the strip above it hide
	cell    atomic.Uint64 // the zoom, float64 bits
	finger  bool          // a finger's press was taken; the event loop's alone
	fitted  chan struct{} // closed once the page has fitted its grid to the zoom
	fitOnce sync.Once

	mu       sync.Mutex
	inflight bool                      // a frame the page has not reported drawn
	own      map[string]func(js.Value) // what takes each of the program's own kinds
}

// OpenPage presents on the page: the font and the zoom go over first, then frames
// as the page takes them. A zoom kept from an earlier visit shows another grid than
// the page measured, so OpenPage returns once the page has fitted it and the
// program starts at the grid on screen. A page lives as long as its worker: nothing
// closes it. The error is a program with no page, as under Node.
func OpenPage(c PageConfig) (*Page, error) {
	if bridge("frame").Type() != js.TypeFunction || bridge("post").Type() != js.TypeFunction {
		return nil, errors.New("terminal: no page: the worker defines no " + jsPage + " with frame and post")
	}
	p := &Page{Frames: NewFrames(nil), events: make(chan Event, pageQueue), barRows: c.BarRows,
		fitted: make(chan struct{}), own: map[string]func(js.Value){}}
	p.cell.Store(math.Float64bits(c.Cell))
	p.keys.Post = p.post
	cols, rows, ok := WindowSize(nil)
	if !ok {
		cols, rows = 80, 24
	}
	p.setGrid(cols, rows)
	js.Global().Get(jsPage).Set("event", js.FuncOf(p.handle))
	if len(c.Font) > 0 {
		PagePost("font", map[string]any{"data": PageBytes(c.Font)})
	}
	PagePost("zoom", map[string]any{"cell": c.Cell, "least": c.CellLeast, "most": c.CellMost,
		"step": c.CellStep, "bar": c.BarRows})
	select {
	case <-p.fitted:
	case <-time.After(fitWait): // a page that never answers starts at its own grid
	}
	return p, nil
}

func (p *Page) Events() <-chan Event { return p.events }

// Size is the grid of whole cells the page holds
func (p *Page) Size() (cols, rows int) {
	g := p.grid.Load()
	return int(g >> 32), int(uint32(g))
}

func (p *Page) setGrid(cols, rows int) {
	p.grid.Store(uint64(max(cols, 1))<<32 | uint64(uint32(max(rows, 1))))
}

// Flush keeps the frame as the newest and hands it to the page if none is on its way
func (p *Page) Flush(cells []Cell, width, height int) {
	p.Frames.Flush(cells, width, height)
	p.present()
}

// SetKeyReport asks for each key's release, which the page's keyups give; a key
// the page has no keyup for, typed on a soft keyboard or the key strip, lets go as
// it presses
func (p *Page) SetKeyReport(report KeyReport, allow KeySource) error {
	p.keys.Ask(report, allow, true)
	return nil
}

func (p *Page) SetPointerMode(enabled, motion bool) {
	p.pointer.Store(enabled)
	PagePost("pointer", map[string]any{"enabled": enabled, "motion": motion})
}

// Covered is the grid's rows at the foot a soft keyboard hides with the strip the
// page draws above it; 0 with no keyboard
func (p *Page) Covered() int { return int(p.covered.Load()) }

// Cell is the zoom the page was left at, a cell's height in CSS pixels, 0 for its own
func (p *Page) Cell() float64 { return math.Float64frombits(p.cell.Load()) }

// Handle has fn take the page's messages of kind, one of the program's own, as the
// page's script posted them. fn runs on the worker's event loop, which it must not
// hold; a message of a kind nothing handles is dropped.
func (p *Page) Handle(kind string, fn func(msg js.Value)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.own[kind] = fn
}

// admit is the pointer mode applied to what the page reports, as a phone's view
// applies it: with the pointer off, a finger still presses on the bar's rows, where
// a program keeps the way back to its pointer, and lifts from a press that was taken
func (p *Page) admit(ev Event) bool {
	switch {
	case ev.MouseAction == MouseActionPress && ev.MouseBtn == MouseBtnLeft:
		_, rows := p.Size()
		p.finger = p.pointer.Load() || ev.Touch && ev.MouseY >= rows-p.barRows
		return p.finger
	case ev.MouseAction == MouseActionRelease && ev.MouseBtn == MouseBtnLeft:
		taken := p.finger || p.pointer.Load()
		p.finger = false
		return taken
	}
	return p.pointer.Load()
}

// present hands the page the newest frame once the one it was handed is drawn
func (p *Page) present() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inflight {
		return
	}
	frame := p.Take()
	if frame == nil {
		return
	}
	p.inflight = true
	js.Global().Get(jsPage).Call("frame", PageBytes(frame))
}

// handle takes what the page sent: a frame drawn, a key going down or up, its keys
// going elsewhere, text, the pointer, a keyboard over it, a zoom, a new size, the
// grid fitted to the zoom OpenPage sent, a request for a whole frame, or a message
// of the program's own
func (p *Page) handle(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return nil
	}
	msg := args[0]
	switch kind := msg.Get("type").String(); kind {
	case "drawn":
		p.mu.Lock()
		p.inflight = false
		p.mu.Unlock()
		p.present()
	case "key":
		if ev, ok := pageKey(msg.Get("key").String(), uint8(msg.Get("mods").Int())); ok {
			if code := msg.Get("code"); code.Type() == js.TypeString && code.String() != "" {
				p.keys.Down(pageKeyCode(code.String()), ev, msg.Get("repeat").Truthy())
			} else {
				p.keys.Tap(ev)
			}
		}
	case "keyup":
		p.keys.Up(pageKeyCode(msg.Get("code").String()))
	case "blur":
		p.keys.Lost()
	case "text":
		// One event, which a program types a character at a time, so a long paste fits the queue
		if text := msg.Get("text").String(); text != "" {
			p.post(Event{Type: EventPaste, Text: text})
		}
	case "mouse":
		if ev, ok := pageMouse(msg.Get("x").Int(), msg.Get("y").Int(), msg.Get("button").Int(),
			msg.Get("action").Int(), uint8(msg.Get("mods").Int()), msg.Get("touch").Truthy()); ok && p.admit(ev) {
			p.post(ev)
		}
	case "covered":
		p.covered.Store(int32(max(msg.Get("rows").Int(), 0)))
	case "zoom":
		p.cell.Store(math.Float64bits(msg.Get("cell").Float()))
	case "resize":
		cols, rows := max(msg.Get("cols").Int(), 1), max(msg.Get("rows").Int(), 1)
		if c, r := p.Size(); c != cols || r != rows {
			p.setGrid(cols, rows)
			p.post(Event{Type: EventResize, Width: cols, Height: rows})
		}
	case "fitted": // after the resize the fit sent, if it changed the grid
		p.fitOnce.Do(func() { close(p.fitted) })
	case "sync":
		p.Sync()
	default:
		p.mu.Lock()
		fn := p.own[kind]
		p.mu.Unlock()
		if fn != nil {
			fn(msg)
		}
	}
	return nil
}

// post queues an event for the program; with the queue full the event is lost, which
// an event loop that must not wait on the program prefers
func (p *Page) post(ev Event) {
	select {
	case p.events <- ev:
	default:
	}
}

// bridge is what the page's worker holds under name in terminalPage, undefined where
// there is no worker, as under Node
func bridge(name string) js.Value {
	if page := js.Global().Get(jsPage); page.Type() == js.TypeObject {
		return page.Get(name)
	}
	return js.Undefined()
}

// PagePost sends the page a message of the program's own kind with its fields, which
// its script handles; a PageBytes among them moves to the page. No page, no message.
func PagePost(kind string, fields map[string]any) {
	if bridge("post").Type() == js.TypeFunction {
		js.Global().Get(jsPage).Call("post", kind, fields)
	}
}

// PageHas reports whether the page's script handles the program's own messages
// named feature, so a program on a page without it does without
func PageHas(feature string) bool {
	f := bridge("features")
	return f.Type() == js.TypeObject && f.Call("includes", feature).Bool()
}

// PageKept is the document the page kept under name from an earlier visit, nil for none
func PageKept(name string) []byte {
	if kept := bridge("kept"); kept.Type() == js.TypeObject {
		if text := kept.Get(name); text.Type() == js.TypeString && text.String() != "" {
			return []byte(text.String())
		}
	}
	return nil
}

// PageKeep hands the page a document to keep under name; for how long is the page's
func PageKeep(name string, doc []byte) {
	PagePost("keep", map[string]any{"name": name, "text": string(doc)})
}

// PageHidden reports a page nobody sees, which a program draws nothing for: its
// frames would wait behind the page's throttled timers
func PageHidden() bool { return bridge("hidden").Truthy() }

// PageBytes copies b into a Uint8Array of its own, whose buffer the worker moves to
// the page instead of copying it again
func PageBytes(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// WindowSize is the grid the page measured, whatever f is; ok is false with no page
func WindowSize(_ *os.File) (w, h int, ok bool) {
	grid := bridge("grid")
	if grid.Type() != js.TypeObject {
		return 0, 0, false
	}
	return grid.Get("cols").Int(), grid.Get("rows").Int(), true
}

// newTerminal is New in a browser: a page is truecolor whatever mode was asked
func newTerminal(ColorMode) Terminal { return &pageTerm{} }

// pageTerm is a Terminal over the page its Init opens; until then Init and Fini
// alone may be called
type pageTerm struct{ Terminal }

func (t *pageTerm) Init() error {
	if t.Terminal == nil {
		p, err := OpenPage(PageConfig{})
		if err != nil {
			return err
		}
		t.Terminal = NewWithHost(p)
	}
	return t.Terminal.Init()
}

func (t *pageTerm) Fini() {
	if t.Terminal != nil {
		t.Terminal.Fini()
	}
}
