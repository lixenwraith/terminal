package terminal

import (
	"slices"
	"strings"
	"sync"
)

// KeyReport is how much a terminal reports of each key
type KeyReport uint8

const (
	KeyReportPresses  KeyReport = iota // presses; a key held down repeats as more presses
	KeyReportReleases                  // presses, repeats and releases, each with its KeyAction
)

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
	// a window or a browser, and builds Events itself, as HostKeys does; only a
	// Terminal over a Host reports it.
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
// keycode); KeyHolds hands its reports on as Events, a release carrying what its
// press carried, so letting go of Shift first still releases 'A', not 'a'. The
// zero value is ready to use; it is not safe for concurrent use.
type KeyHolds struct {
	held []heldKey // oldest press first
}

type heldKey struct {
	code  uint32
	press Event
}

// Held reports whether the key code names is down: a source that repeats a held
// key as more presses, as the consoles do, tells its repeats apart by it
func (h *KeyHolds) Held(code uint32) bool {
	return h.find(code) >= 0
}

func (h *KeyHolds) find(code uint32) int {
	return slices.IndexFunc(h.held, func(k heldKey) bool { return k.code == code })
}

// Report hands on one report of the key code names through send, ev the key as
// the source reads it, its Action set: a press starts a hold, a release ends
// it, a repeat passes as it is. A report the holds contradict means the source
// lost one: a press of a key held first ends that hold with a Lost release, a
// repeat of a key not held is a press, and a release of one is dropped.
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

// HostKeys is the key report of a host that reads its platform's own key
// events, a window, a page or a phone, and posts them as a terminal would:
// presses until a game asks for releases, then each key's press, repeats and
// release, paired by the host's code for the physical key. Post takes the events
// it makes and is set before use; any goroutine may call any method.
type HostKeys struct {
	Post func(Event)

	mu       sync.Mutex
	releases bool
	holds    KeyHolds
}

// Ask answers a game's SetKeyReport with an EventKeyReport: releases when it asks a
// host for them and this one can tell a release from a repeat, presses otherwise,
// which ends every hold as lost
func (k *HostKeys) Ask(report KeyReport, allow KeySource, can bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.releases = can && report == KeyReportReleases && allow&KeySourceHost != 0
	answer := Event{Type: EventKeyReport}
	if k.releases {
		answer.Report, answer.Source = KeyReportReleases, KeySourceHost
	} else {
		k.holds.ReleaseAll(k.Post)
	}
	k.Post(answer)
}

// Down is the key code names going down as ev, or repeating while held; with
// releases reported its Up ends the hold
func (k *HostKeys) Down(code uint32, ev Event, repeat bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.releases {
		k.Post(ev)
		return
	}
	if repeat {
		ev.Action = KeyRepeat
	}
	k.holds.Report(code, ev, k.Post)
}

// Up is the key code names coming up
func (k *HostKeys) Up(code uint32) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.releases {
		k.holds.Report(code, Event{Type: EventKey, Action: KeyRelease}, k.Post)
	}
}

// Tap is a key with no release of its own to come, a soft keyboard's text or a key
// the host made: with releases reported it lets go as it presses, lost
func (k *HostKeys) Tap(ev Event) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Post(ev)
	if k.releases {
		ev.Action, ev.Lost = KeyRelease, true
		k.Post(ev)
	}
}

// Lost ends every hold as lost: the host's keys went to something else
func (k *HostKeys) Lost() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.holds.ReleaseAll(k.Post)
}

// Held reports whether the key code names is down, for a host whose keyboard
// repeats a held key as more presses
func (k *HostKeys) Held(code uint32) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.holds.Held(code)
}
