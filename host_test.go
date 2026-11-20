package terminal

import (
	"slices"
	"testing"
	"time"

	"github.com/lixenwraith/color"
)

// fakeHost records what a Terminal asks of its host; its events are unbuffered,
// so a send returns once the Terminal is done with the event before it
type fakeHost struct {
	events          chan Event
	cells           []Cell
	width, height   int
	enabled, motion bool
	closed          int
}

func (h *fakeHost) ColorMode() ColorMode                    { return ColorModeTrueColor }
func (h *fakeHost) Sync()                                   {}
func (h *fakeHost) Size() (int, int)                        { return 3, 2 }
func (h *fakeHost) Blocked() time.Duration                  { return 0 }
func (h *fakeHost) Events() <-chan Event                    { return h.events }
func (h *fakeHost) SetKeyReport(KeyReport, KeySource) error { return nil }
func (h *fakeHost) Close()                                  { h.closed++ }

func (h *fakeHost) Flush(cells []Cell, width, height int) {
	h.cells, h.width, h.height = slices.Clone(cells), width, height
}

func (h *fakeHost) SetPointerMode(enabled, motion bool) { h.enabled, h.motion = enabled, motion }

// TestATerminalOverAHostIsItsScreenAndItsEvents: PollEvent hands on what the host
// reports, a synthetic event first; a resize waits in ResizeChan for either reader,
// the newest alone; a mouse mode is the host's pointer mode and Clear a frame of
// blanks at its size; Fini closes the host once, Init or none; after it, or once the
// host's events end, PollEvent is EventClosed, behind what the host had sent
func TestATerminalOverAHostIsItsScreenAndItsEvents(t *testing.T) {
	h := &fakeHost{events: make(chan Event)}
	term := NewWithHost(h)
	if term.Init() != nil || term.Init() != nil {
		t.Fatal("Init failed over a host that is open")
	}
	resize := func(w, h int) Event { return Event{Type: EventResize, Width: w, Height: h} }
	poll := func(n int) (polled []Event) {
		for range n {
			polled = append(polled, term.PollEvent())
		}
		return polled
	}

	h.events <- key('a')
	h.events <- key('b') // a is queued now
	for range 8 {
		term.PostEvent(key('s'))
	}
	if got := poll(10); !slices.Equal(got, append(slices.Repeat([]Event{key('s')}, 8), key('a'), key('b'))) {
		t.Fatalf("polled %+v, want every synthetic s, then the host's a and b", got)
	}
	h.events <- resize(5, 4)
	h.events <- resize(7, 6)
	h.events <- key('c')
	if re := <-term.ResizeChan(); re != (ResizeEvent{Width: 7, Height: 6}) {
		t.Fatalf("ResizeChan held %+v, want the newest resize", re)
	}
	passed := term.PollEvent()
	h.events <- resize(9, 8)
	if polled := term.PollEvent(); passed != key('c') || polled != resize(9, 8) {
		t.Fatalf("polled %+v then %+v, want c then the next resize", passed, polled)
	}

	for _, m := range []MouseMode{MouseModeClick | MouseModeDrag, MouseModeClick | MouseModeMotion, MouseModeNone} {
		term.SetMouseMode(m)
		if h.enabled != (m != MouseModeNone) || h.motion != (m&MouseModeMotion != 0) {
			t.Fatalf("mouse mode %b set the pointer %v, motion %v", m, h.enabled, h.motion)
		}
	}
	bg := color.RGB{R: 1, G: 2, B: 3}
	term.Clear(bg)
	if !slices.Equal(h.cells, slices.Repeat([]Cell{{Rune: ' ', Bg: bg}}, 6)) || h.width != 3 || h.height != 2 {
		t.Fatalf("Clear flushed %dx%d of %+v", h.width, h.height, h.cells)
	}

	for _, c := range "defg" {
		h.events <- key(c)
	}
	term.Fini()
	term.Fini()
	never := &fakeHost{}
	NewWithHost(never).Fini()
	if h.closed != 1 || never.closed != 1 {
		t.Fatalf("Fini closed the host %d times, and one never initialized %d", h.closed, never.closed)
	}
	if got := poll(5); !slices.Equal(got, []Event{key('d'), key('e'), key('f'), key('g'), {Type: EventClosed}}) {
		t.Fatalf("after Fini polled %+v, want what the host had sent, then EventClosed", got)
	}
	ended := &fakeHost{events: make(chan Event)}
	close(ended.events)
	over := NewWithHost(ended)
	over.Init()
	if ev := over.PollEvent(); ev.Type != EventClosed {
		t.Fatalf("polled %+v over a host whose events had ended", ev)
	}
}
