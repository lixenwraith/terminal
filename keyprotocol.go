package terminal

import (
	"strconv"
	"time"
)

// The terminal's key event protocol (CSI u with progressive enhancement, as
// Alacritty 0.13 and later implement it): asked for flags, the terminal
// reports every key as an escape code with its event type, press, repeat or
// release, and the text it types.
const (
	protoDisambiguate = 1 << iota // Escape, Alt and Ctrl keys as escape codes
	protoEventTypes               // repeats and releases
	protoAlternates               // the shifted key beside the key
	protoAllKeys                  // text keys as escape codes too, so they release
	protoText                     // the text a key types, in its code

	// protoFlags are the flags pushed, and protoNeed those a terminal must
	// confirm before the reader decodes by them: the text keeps every layout's
	// characters as the terminal types them
	protoFlags = protoDisambiguate | protoEventTypes | protoAlternates | protoAllKeys | protoText
	protoNeed  = protoEventTypes | protoAllKeys | protoText
)

var (
	csiKeyQuery = []byte("\x1b[?u")                                 // the flags in force, answered CSI ? flags u
	csiKeyPush  = []byte("\x1b[>" + strconv.Itoa(protoFlags) + "u") // over the flags in force
	csiKeyPop   = []byte("\x1b[<u")                                 // back to the flags before the push
	csiDA1      = []byte("\x1b[c")                                  // primary device attributes, which every terminal answers
	csiFocusOn  = []byte("\x1b[?1004h")
	csiFocusOff = []byte("\x1b[?1004l")
)

// protoTimeout bounds an exchange with the terminal; one that answers nothing
// by then has no protocol to use
var protoTimeout = 2 * time.Second

// protoExchange is a request to the terminal awaiting its answer, which the
// DA1 reply after it closes: terminals answer in order
type protoExchange uint8

const (
	exchangeNone  protoExchange = iota
	exchangeProbe               // does the terminal have the protocol?
	exchangePush                // pushed protoFlags: what did they come to?
	exchangePop                 // popped: reports go on until the terminal has it
)

// keyAsk is what a SetKeyReport call asked for
type keyAsk struct {
	report KeyReport
	allow  KeySource
}

// keySourcer is a backend that has sources of key releases. Only the
// package's own are: a Terminal over a caller's backend reports presses.
type keySourcer interface {
	keySources() KeySource
}

// backendKeySources is what b offers beyond presses
func backendKeySources(b Backend) KeySource {
	if s, ok := b.(keySourcer); ok {
		return s.keySources()
	}
	return 0
}

// askKeys hands an ask to the reader; with no source, every ask comes to
// presses at once
func (r *inputReader) askKeys(report KeyReport, allow KeySource) {
	if r.sources == 0 {
		r.sendEvent(Event{Type: EventKeyReport, Report: KeyReportPresses})
		return
	}
	a := keyAsk{report, allow & r.sources}
	for {
		select {
		case r.asks <- a:
			return
		default:
		}
		select { // an ask the reader has not taken yet gives way to this one
		case <-r.asks:
		default:
		}
	}
}

// serviceKeys takes the latest ask, gives up on an exchange the terminal has
// not answered in time, and moves toward the ask; the reader calls it between
// reads
func (r *inputReader) serviceKeys() {
	select {
	case a := <-r.asks:
		r.want, r.answer = a, true
	default:
	}
	if r.exchange != exchangeNone && time.Now().After(r.deadline) {
		r.exchangeDone(false)
	}
	r.reconcile()
}

// target is the source the ask wants of those to be had, the one in use while
// it is still allowed
func (r *inputReader) target() KeySource {
	a := r.want
	switch {
	case a.report == KeyReportPresses:
		return 0
	case r.source != 0 && a.allow&r.source != 0:
		return r.source
	case a.allow&KeySourceConsole != 0 && r.consoleHas >= 0:
		return KeySourceConsole
	case a.allow&KeySourceProtocol != 0 && r.protoHas >= 0:
		return KeySourceProtocol
	}
	return 0
}

// reconcile moves toward the ask while no exchange is in flight, and answers
// it once there
func (r *inputReader) reconcile() {
	for r.answer && r.exchange == exchangeNone {
		switch target := r.target(); {
		case target == r.source:
			r.answer = false
			r.sendEvent(Event{Type: EventKeyReport, Report: r.report, Source: r.source})
		case r.source != 0:
			r.leave()
		case target == KeySourceConsole:
			r.rawConsole()
		case r.protoHas == 0:
			r.request(exchangeProbe, false, csiKeyQuery, csiDA1)
		default:
			r.decoding = true
			r.request(exchangePush, true, csiKeyPush, csiKeyQuery, csiDA1)
		}
	}
}

// request writes an exchange's requests and waits for the answer; pushed is
// whether the protocol's flags are pushed once they are written. A terminal
// closing takes no requests, and the protocol is then none.
func (r *inputReader) request(x protoExchange, pushed bool, seqs ...[]byte) {
	var seq []byte
	for _, s := range seqs {
		seq = append(seq, s...)
	}
	if r.write == nil || !r.write(seq, pushed) {
		r.protoHas, r.decoding = -1, false
		return
	}
	r.exchange, r.deadline, r.protoReported = x, time.Now().Add(protoTimeout), -1
}

// protoReply takes the flags a terminal reports in force
func (r *inputReader) protoReply(flags int) {
	if r.exchange != exchangeNone {
		r.protoReported = flags
	}
}

// exchangeDone closes the exchange in flight: answered by the DA1 reply after
// it, or not in time
func (r *inputReader) exchangeDone(answered bool) {
	x := r.exchange
	if x == exchangeNone {
		return
	}
	r.exchange = exchangeNone
	switch x {
	case exchangeProbe:
		r.protoHas = -1
		if answered && r.protoReported >= 0 {
			r.protoHas = 1
		}
	case exchangePush:
		if answered && r.protoReported&protoNeed == protoNeed {
			// Keys held across the switch were reported as presses; their next
			// repeat starts their hold
			r.holds = KeyHolds{}
			r.source, r.report, r.focus = KeySourceProtocol, KeyReportReleases, true
			r.write(csiFocusOn, true)
		} else {
			r.protoHas = -1
			r.request(exchangePop, false, csiKeyPop, csiDA1)
		}
	case exchangePop:
		r.decoding = false
	}
	r.reconcile()
}

// leave gives up the source in use, ending its holds first
func (r *inputReader) leave() {
	r.holds.ReleaseAll(r.sendEvent)
	source := r.source
	r.source, r.report, r.focus = 0, KeyReportPresses, false
	switch source {
	case KeySourceProtocol:
		r.request(exchangePop, false, csiKeyPop, csiFocusOff, csiDA1)
	case KeySourceConsole:
		r.rawKeys(false)
	}
}

// protoNamed sets the codes of keys the protocol reports in their legacy form
// (arrows, Home, F1, ...) apart from Unicode code points
const protoNamed = 0x110000

// tildeKeys are the keys reported as CSI number ; modifiers ~
var tildeKeys = map[int32]Key{
	1: KeyHome, 2: KeyInsert, 3: KeyDelete, 4: KeyEnd, 5: KeyPageUp, 6: KeyPageDown, 7: KeyHome, 8: KeyEnd,
	11: KeyF1, 12: KeyF2, 13: KeyF3, 14: KeyF4, 15: KeyF5, 17: KeyF6, 18: KeyF7, 19: KeyF8,
	20: KeyF9, 21: KeyF10, 23: KeyF11, 24: KeyF12,
}

// letterKeys are the keys reported as CSI 1 ; modifiers letter
var letterKeys = map[byte]Key{
	'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'H': KeyHome, 'F': KeyEnd,
	'P': KeyF1, 'Q': KeyF2, 'S': KeyF4, // F3 as CSI R would read as a cursor position report
}

// Keypad keys by protocol code: those that type a character, and those that
// move as their main-block twins with Num Lock off
var (
	keypadRunes = map[int32]rune{
		57399: '0', 57400: '1', 57401: '2', 57402: '3', 57403: '4', 57404: '5', 57405: '6', 57406: '7',
		57407: '8', 57408: '9', 57409: '.', 57410: '/', 57411: '*', 57412: '-', 57413: '+', 57415: '=',
		57416: ',',
	}
	keypadKeys = map[int32]Key{
		57414: KeyEnter, 57417: KeyLeft, 57418: KeyRight, 57419: KeyUp, 57420: KeyDown, 57421: KeyPageUp,
		57422: KeyPageDown, 57423: KeyHome, 57424: KeyEnd, 57425: KeyInsert, 57426: KeyDelete,
	}
)

// keyParams are a key report's parameters: up to three fields split by ';',
// each of subfields split by ':', an absent one -1
type keyParams [3][8]int32

// parseKeyParams reads a key report's parameters; ok is false for anything
// else, a private marker or a value past Unicode among them
func parseKeyParams(p []byte) (v keyParams, ok bool) {
	for i := range v {
		for j := range v[i] {
			v[i][j] = -1
		}
	}
	f, s := 0, 0
	for _, b := range p {
		switch {
		case b >= '0' && b <= '9':
			n := max(v[f][s], 0)*10 + int32(b-'0')
			if n > 0x10ffff {
				return v, false
			}
			v[f][s] = n
		case b == ':':
			if s++; s == len(v[f]) {
				return v, false
			}
		case b == ';':
			if f, s = f+1, 0; f == len(v) {
				return v, false
			}
		default:
			return v, false
		}
	}
	return v, true
}

// protoKey decodes a key the protocol reports, CSI params final, and sends what
// it means at the level in force. It is false for a sequence that is no key
// report, which the legacy tables then read.
//
// Each press reads as the key a legacy terminal sends for it reads (Alacritty's
// encoding is the reference), so a game sees the same keys with the protocol
// on: Shift goes into the character, Ctrl with a character becomes its control
// key, Alt stays a modifier, and Super and the locks are not reported. Keys a
// legacy terminal sends nothing for (lone modifiers, media keys, F13 and up)
// are dropped.
func (r *inputReader) protoKey(params []byte, final byte) bool {
	v, ok := parseKeyParams(params)
	if !ok {
		return false
	}
	num, mods := v[0][0], max(v[1][0], 1)-1
	action := KeyPress
	switch v[1][1] {
	case 2:
		action = KeyRepeat
	case 3:
		action = KeyRelease
	}
	ev := Event{Type: EventKey, Action: action}
	var code uint32
	switch final {
	case 'u':
		if num < 0 {
			return false
		}
		var text []rune
		for _, c := range v[2] {
			if c >= 0 && printable(rune(c)) {
				text = append(text, rune(c))
			}
		}
		if !protoCodeKey(num, v[0][1], mods, text, &ev) {
			// A key with no legacy key, or text with no key: its text is taps
			for _, t := range text {
				r.sendKey(0, Event{Type: EventKey, Key: KeyRune, Rune: t})
			}
			return true
		}
		code = uint32(num)
		if ev.Key == KeyRune && len(text) > 1 {
			defer func() {
				for _, t := range text[1:] {
					r.sendKey(0, Event{Type: EventKey, Key: KeyRune, Rune: t})
				}
			}()
		}
	case '~':
		if ev.Key = tildeKeys[num]; ev.Key == KeyNone {
			return true // F13 and up, keypad Begin: no legacy key
		}
		ev.Modifiers = keyModifiers(mods)
		code = protoNamed + uint32(ev.Key)
	default:
		if num > 1 {
			return false
		}
		if ev.Key = letterKeys[final]; ev.Key == KeyNone {
			return final == 'E' // keypad Begin
		}
		ev.Modifiers = keyModifiers(mods)
		code = protoNamed + uint32(ev.Key)
	}
	r.sendKey(code, ev)
	return true
}

// protoCodeKey reads the key a CSI u report's code names into ev, as a legacy
// terminal reports it: false for a code no legacy key has
func protoCodeKey(num, shifted, mods int32, text []rune, ev *Event) bool {
	alt := Modifier(0)
	if mods&2 != 0 {
		alt = ModAlt
	}
	shift, ctrl := mods&1 != 0, mods&4 != 0
	switch {
	case num == 27:
		ev.Key, ev.Modifiers = KeyEscape, alt
	case num == 13:
		ev.Key, ev.Modifiers = KeyEnter, alt
	case num == 9 && shift:
		ev.Key, ev.Modifiers = KeyBacktab, ModShift|alt
	case num == 9:
		ev.Key, ev.Modifiers = KeyTab, alt
	case num == 127, num == 8:
		ev.Key, ev.Modifiers = KeyBackspace, alt
	case keypadKeys[num] != KeyNone:
		ev.Key = keypadKeys[num]
		if ev.Key == KeyEnter {
			ev.Modifiers = alt
		} else {
			ev.Modifiers = keyModifiers(mods)
		}
	default:
		ch, ok := keypadRunes[num]
		if !ok {
			ch = rune(num)
		}
		if !ok && (num >= 0xe000 && num <= 0xf8ff || !printable(ch)) {
			return false // the protocol's own keys, and codes no key types
		}
		if shift && shifted > 0 {
			ch = rune(shifted)
		}
		if ctrl {
			if k, ok := ControlKey(ch); ok {
				ev.Key, ev.Modifiers = k, alt
				return true
			}
			if k, ok := ControlKey(rune(num)); ok {
				ev.Key, ev.Modifiers = k, alt
				return true
			}
		}
		if len(text) > 0 {
			ch = text[0]
		}
		ev.Key, ev.Rune, ev.Modifiers = KeyRune, ch, alt
		if ctrl {
			ev.Modifiers |= ModCtrl
		}
	}
	return true
}

// keyModifiers are the protocol's modifier bits a legacy key reports
func keyModifiers(mods int32) Modifier {
	return Modifier(mods & int32(ModShift|ModAlt|ModCtrl))
}

// printable is whether r types something: no control character, no surrogate
func printable(r rune) bool {
	return r >= 0x20 && (r < 0x7f || r > 0x9f) && (r < 0xd800 || r > 0xdfff) && r <= 0x10ffff
}
