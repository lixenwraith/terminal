package terminal

import (
	"slices"
	"sync"
	"time"

	"github.com/lixenwraith/color"
)

// Host is a screen with its player's keys and pointer that is open already and
// speaks in cells and Events itself: a window, a page, a phone's view. Its
// Events carry a resize as an EventResize, and Size is the grid it then holds.
type Host interface {
	Surface
	Size() (cols, rows int)
	Blocked() time.Duration // how long it has held a frame it has not drawn
	Events() <-chan Event
	// SetPointerMode selects the pointer events reported: none, buttons and
	// drags, or every motion as well
	SetPointerMode(enabled, motion bool)
	// SetKeyReport is answered with an EventKeyReport on Events, as HostKeys.Ask posts
	SetKeyReport(report KeyReport, allow KeySource) error
}

// hostTerm is a Host as a Terminal: Size, ColorMode, Blocked, Flush, Sync and
// SetKeyReport are the embedded host's
type hostTerm struct {
	Host

	events      chan Event // the host's, its resizes aside
	resizeCh    chan ResizeEvent
	syntheticCh chan Event
	quit        chan struct{}
	pumped      chan struct{} // pump returned

	mu        sync.Mutex
	pumping   bool
	finalized bool
}

// NewWithHost creates a Terminal over h, open already: Init opens nothing, and
// Fini calls h's Close when it has one. A host draws no cursor of its own, and
// its paste is one EventPaste whatever the paste mode
func NewWithHost(h Host) Terminal {
	return &hostTerm{
		Host:        h,
		events:      make(chan Event, 256),
		resizeCh:    make(chan ResizeEvent, 1),
		syntheticCh: make(chan Event, 16),
		quit:        make(chan struct{}),
		pumped:      make(chan struct{}),
	}
}

// Init starts taking the host's events
func (t *hostTerm) Init() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.pumping && !t.finalized {
		t.pumping = true
		go t.pump()
	}
	return nil
}

// pump moves the host's events to PollEvent, a resize by way of ResizeChan as a
// tty's goes. A full queue drops an event, as an input reader's does, so a
// resize never waits behind keys nobody polls
func (t *hostTerm) pump() {
	defer close(t.pumped)
	for {
		select {
		case <-t.quit:
			return
		case ev, ok := <-t.Events():
			switch {
			case !ok:
				return
			case ev.Type == EventResize:
				postResize(t.resizeCh, ResizeEvent{Width: ev.Width, Height: ev.Height})
			default:
				select {
				case t.events <- ev:
				default:
				}
			}
		}
	}
}

// Fini stops taking the host's events, then closes a host that can be closed
func (t *hostTerm) Fini() {
	t.mu.Lock()
	if t.finalized {
		t.mu.Unlock()
		return
	}
	t.finalized = true
	pumping := t.pumping
	t.mu.Unlock()

	close(t.quit)
	if pumping {
		<-t.pumped
	}
	if c, ok := t.Host.(interface{ Close() }); ok {
		c.Close()
	}
}

func (t *hostTerm) ResizeChan() <-chan ResizeEvent { return t.resizeCh }

// Clear is a frame of blank cells: a host takes frames and nothing else
func (t *hostTerm) Clear(bg color.RGB) {
	cols, rows := t.Size()
	t.Flush(slices.Repeat([]Cell{{Rune: ' ', Bg: bg}}, max(cols*rows, 0)), cols, rows)
}

func (t *hostTerm) SetCursorVisible(bool) {}

func (t *hostTerm) MoveCursor(int, int) {}

// PollEvent blocks until the next event, a synthetic one first; once Fini has
// begun or the host's events have ended it is EventClosed, after those queued
func (t *hostTerm) PollEvent() Event {
	select {
	case ev := <-t.syntheticCh:
		return ev
	default:
	}
	select {
	case ev := <-t.syntheticCh:
		return ev
	case ev := <-t.events:
		return ev
	case re := <-t.resizeCh:
		return Event{Type: EventResize, Width: re.Width, Height: re.Height}
	case <-t.quit:
	case <-t.pumped:
	}
	select {
	case ev := <-t.events:
		return ev
	default:
		return Event{Type: EventClosed}
	}
}

// PostEvent injects a synthetic event, dropped when the queue of them is full
func (t *hostTerm) PostEvent(ev Event) {
	select {
	case t.syntheticCh <- ev:
	default:
	}
}

func (t *hostTerm) SetMouseMode(mode MouseMode) error {
	t.SetPointerMode(mode != MouseModeNone, mode&MouseModeMotion != 0)
	return nil
}

func (t *hostTerm) SetPasteMode(bool) error { return nil }
