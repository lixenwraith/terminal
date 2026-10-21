package terminal

import (
	"slices"
	"strings"
)

// KeyReport is how much a terminal reports of each key
type KeyReport uint8

const (
	KeyReportPresses  KeyReport = iota // presses; a key held down repeats as more presses
	KeyReportReleases                  // presses, repeats and releases, each with its KeyAction
)

// String returns the report's name
func (r KeyReport) String() string {
	if r == KeyReportReleases {
		return "Releases"
	}
	return "Presses"
}

// KeySource is where key releases come from (bitmask). SetKeyReport tries the
// sources a game allows in the order below; a terminal with none of them
// reports presses, as every terminal does until a game asks.
type KeySource uint8

const (
	// KeySourceConsole is the kernel console's own keyboard, switched to raw
	// keycodes while releases are asked for: the Linux console on the VT the
	// game runs on, and FreeBSD's vt(4). Never over ssh, where the keyboard is
	// the far terminal's.
	KeySourceConsole KeySource = 1 << iota

	// KeySourceProtocol is the terminal's key event protocol (CSI u with event
	// types), found by asking the terminal whether it has one: Alacritty 0.13
	// and later, and any terminal that answers the same. Over ssh too.
	KeySourceProtocol

	// KeySourceHost is a host that reads its platform's own key events, such as
	// a window or a browser, and builds Events itself; no Terminal reports it.
	KeySourceHost
)

// String returns the sources' names, joined by |
func (s KeySource) String() string {
	var names []string
	for _, n := range []struct {
		s    KeySource
		name string
	}{{KeySourceConsole, "Console"}, {KeySourceProtocol, "Protocol"}, {KeySourceHost, "Host"}} {
		if s&n.s != 0 {
			names = append(names, n.name)
		}
	}
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, "|")
}

// maxHolds bounds the keys KeyHolds keeps down at once, past any keyboard's
// rollover, so a source that never releases can't grow it without end
const maxHolds = 32

// KeyHolds pairs key releases with their presses. A source names each physical
// key by a code of its own (a console keycode, a protocol key number, a host's
// keycode) and reports presses, repeats and releases of it; KeyHolds hands each
// on as an Event, a release carrying what its press carried, so letting go of
// Shift first still releases 'A', not 'a'. The zero value is ready to use. It
// is not safe for concurrent use.
type KeyHolds struct {
	held []heldKey // oldest press first
}

type heldKey struct {
	code  uint32
	press Event
}

// Held reports whether the key code names is down
func (h *KeyHolds) Held(code uint32) bool {
	return h.find(code) >= 0
}

func (h *KeyHolds) find(code uint32) int {
	return slices.IndexFunc(h.held, func(k heldKey) bool { return k.code == code })
}

// Report hands on one report of the key code names through send: ev is the key
// as the source reads it, its Action set. A press starts a hold and a release
// ends it, carrying the press's Key, Rune and Modifiers; a repeat passes as it
// is. A report the holds contradict means the source lost one, as a terminal's
// own mode swallows keys: a press of a key held first ends that hold with a
// Lost release, a repeat of a key not held is a press, and a release of a key
// not held is dropped. A source that repeats a held key as more presses, as the
// consoles do, asks Held to tell its repeats apart.
func (h *KeyHolds) Report(code uint32, ev Event, send func(Event)) {
	i := h.find(code)
	switch {
	case ev.Action == KeyRelease:
		if i >= 0 {
			send(h.end(i, false))
		}
		return
	case ev.Action == KeyRepeat && i >= 0:
		send(ev)
		return
	case i >= 0:
		send(h.end(i, true))
	case len(h.held) == maxHolds:
		send(h.end(0, true))
	}
	ev.Action = KeyPress
	h.held = append(h.held, heldKey{code, ev})
	send(ev)
}

// ReleaseAll ends every hold, sending a Lost release for each, oldest first:
// focus left, the console switched away, or releases stopped
func (h *KeyHolds) ReleaseAll(send func(Event)) {
	for len(h.held) > 0 {
		send(h.end(0, true))
	}
}

// end removes hold i and returns its release
func (h *KeyHolds) end(i int, lost bool) Event {
	ev := h.held[i].press
	h.held = slices.Delete(h.held, i, i+1)
	ev.Action, ev.Lost = KeyRelease, lost
	return ev
}
