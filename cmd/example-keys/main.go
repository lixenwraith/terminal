// keys — shows each key event the terminal reports, for trying key releases
// where a game runs: a Linux or FreeBSD console, Alacritty, ssh, tmux.
//
// Keys: F2 asks for releases or gives them back · Ctrl+C quits
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lixenwraith/terminal"
)

func main() {
	term := terminal.New()

	// Raw-mode safety net: restore the terminal before re-panicking.
	defer func() {
		if r := recover(); r != nil {
			terminal.EmergencyReset(os.Stdout)
			panic(r)
		}
	}()

	if err := term.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "terminal init:", err)
		os.Exit(1)
	}
	defer term.Fini()

	v := &viewer{term: term}
	v.ask(true)
	for {
		v.draw()
		ev := term.PollEvent()
		switch ev.Type {
		case terminal.EventClosed, terminal.EventError:
			return
		case terminal.EventKeyReport:
			v.report = ev.Report.String()
			if ev.Source != 0 {
				v.report += " from " + ev.Source.String()
			}
			v.log("  report: " + v.report)
		case terminal.EventFocus:
			v.log(fmt.Sprintf("  focus: %v", ev.Focused))
		case terminal.EventKey:
			v.log(describe(ev))
			if ev.Action == terminal.KeyRelease {
				continue
			}
			switch ev.Key {
			case terminal.KeyCtrlC:
				return
			case terminal.KeyF2:
				if ev.Action == terminal.KeyPress {
					v.ask(!v.releases)
				}
			}
		}
	}
}

// viewer is the screen: what the terminal reports and the events so far
type viewer struct {
	term     terminal.Terminal
	releases bool
	report   string
	lines    []string
}

// ask asks for releases, from the console or the terminal's protocol, or
// gives them back
func (v *viewer) ask(releases bool) {
	v.releases, v.report = releases, "asking"
	if releases {
		v.term.SetKeyReport(terminal.KeyReportReleases, terminal.KeySourceConsole|terminal.KeySourceProtocol)
		return
	}
	v.term.SetKeyReport(terminal.KeyReportPresses, 0)
}

// log adds a line, keeping the last screenful
func (v *viewer) log(line string) {
	v.lines = append(v.lines, line)
	if len(v.lines) > 500 {
		v.lines = v.lines[len(v.lines)-500:]
	}
}

// draw shows the report above the events, the latest at the bottom
func (v *viewer) draw() {
	w, h := v.term.Size()
	if w <= 0 || h <= 0 {
		return
	}
	cells := make([]terminal.Cell, w*h)
	text := func(y int, s string, attr terminal.Attr) {
		x := 0
		for _, r := range s {
			if x >= w {
				break
			}
			cells[y*w+x] = terminal.Cell{Rune: r, Attrs: attr | terminal.AttrFgDefault | terminal.AttrBgDefault}
			x++
		}
		for ; x < w; x++ {
			cells[y*w+x] = terminal.Cell{Rune: ' ', Attrs: attr | terminal.AttrFgDefault | terminal.AttrBgDefault}
		}
	}
	text(0, "keys: "+v.report, terminal.AttrBold|terminal.AttrReverse)
	if h > 1 {
		text(1, "F2 asks for releases or gives them back · Ctrl+C quits", terminal.AttrDim)
	}
	shown := v.lines[max(0, len(v.lines)-(h-2)):]
	for y := 2; y < h; y++ {
		line := ""
		if i := y - 2; i < len(shown) {
			line = shown[i]
		}
		text(y, line, terminal.AttrNone)
	}
	v.term.Flush(cells, w, h)
}

// describe is a key event in one line: its action, key, modifiers and
// whether its release was made up for a hold whose own can't arrive
func describe(ev terminal.Event) string {
	key := terminal.KeyName(ev.Key)
	if ev.Key == terminal.KeyRune {
		key = fmt.Sprintf("%q", ev.Rune)
	}
	var mods []string
	for _, m := range []struct {
		mod  terminal.Modifier
		name string
	}{{terminal.ModCtrl, "ctrl"}, {terminal.ModAlt, "alt"}, {terminal.ModShift, "shift"}} {
		if ev.Modifiers&m.mod != 0 {
			mods = append(mods, m.name)
		}
	}
	line := fmt.Sprintf("%-8v %s", ev.Action, strings.Join(append(mods, key), "+"))
	if ev.Lost {
		line += "  (lost)"
	}
	return line
}
