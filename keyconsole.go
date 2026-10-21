package terminal

// The console source: a kernel console's keyboard switched to raw keycodes.
// Each key types what the console types for it cooked, read as a legacy
// terminal's bytes are, and is held by its keycode.

// keyConsole is a kernel console whose keyboard the reader can switch to raw
// keycodes: the Linux console, FreeBSD's vt(4). The reader calls it from its
// goroutine; close comes from Fini, on another.
type keyConsole interface {
	// prepare reads what translating keys needs and readies the switch: the
	// keymap, the guard, output a Scroll Lock stopped. False when the console
	// can't go raw.
	prepare() bool

	// raw switches the keyboard to raw keycodes, false when the console refuses
	raw() bool

	// cooked switches the keyboard back to the mode raw found
	cooked()

	// keys decodes raw keycodes, keeping a partial one for the next call, and
	// reports each key event with what the console types for it
	keys(data []byte, key func(code uint32, action KeyAction, typed []byte))

	// front is whether the console is the one on screen, the one the
	// keyboard types into
	front() bool

	// close switches the keyboard back if it is raw and ends the guard
	close()
}

// consoleBackend is a backend on a kernel console
type consoleBackend interface {
	keyConsole() keyConsole
}

// backendConsole is the console b runs on, nil for none
func backendConsole(b Backend) keyConsole {
	if c, ok := b.(consoleBackend); ok {
		return c.keyConsole()
	}
	return nil
}

// rawConsole switches the console's keyboard raw for releases; a console that
// refuses is not asked again
func (r *inputReader) rawConsole() {
	// An ESC waiting for its timeout was Escape, pressed before the switch
	if ev, ok := loneEscape[string(r.buf)]; ok {
		r.sendKey(0, ev)
	}
	r.buf = r.buf[:0]
	if r.rawKeys == nil || !r.rawKeys(true) {
		r.consoleHas = -1
		return
	}
	r.holds = KeyHolds{}
	r.source, r.report = KeySourceConsole, KeyReportReleases
}

// consoleRead reads raw keycodes, and ends every hold while another console
// has the keyboard: their releases go there
func (r *inputReader) consoleRead(data []byte) {
	if len(data) > 0 {
		r.console.keys(data, r.consoleKey)
	}
	if !r.console.front() {
		r.holds.ReleaseAll(r.sendEvent)
	}
}

// consoleKey sends a key event as what the console types for it reads: the
// last key held by the keycode, any before it as taps, as a dead key's
// character the key types before its own
func (r *inputReader) consoleKey(code uint32, action KeyAction, typed []byte) {
	if action == KeyRelease {
		r.sendKey(code, Event{Type: EventKey, Action: KeyRelease})
		return
	}
	var last Event
	have := false
	r.typedKeys(typed, func(ev Event) {
		if have {
			r.sendKey(0, last)
		}
		last, have = ev, true
	})
	if have {
		last.Action = action
		r.sendKey(code, last)
	}
}

// typedKeys reads what a console types for one key as a legacy terminal's
// input reads, the key whole: an ESC it ends with is Escape
func (r *inputReader) typedKeys(typed []byte, key func(Event)) {
	if len(typed) == 0 {
		return
	}
	p := r.typer
	if p == nil {
		// A parser of its own: no paste, no protocol, no stop
		p = &inputReader{eventCh: make(chan Event, 512)}
		r.typer = p
	}
	p.dropCSI = false
	n := p.parseInput(typed)
	if ev, ok := loneEscape[string(typed[n:])]; ok {
		p.sendKey(0, ev)
	}
	for {
		select {
		case ev := <-p.eventCh:
			if ev.Type == EventKey && ev.Key != KeyNone {
				key(ev)
			}
		default:
			return
		}
	}
}
