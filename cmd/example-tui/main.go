// fv, file voyager: an lf / midnight-commander / xxd hybrid, the demo of the
// terminal, tui and color packages. Three panes: the parent directory, the
// current one and a preview; Enter on a file opens a hex viewer with a byte
// cursor and search. ? lists the keys, q quits, Q quits into a shell where it was.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lixenwraith/terminal"
)

const tickInterval = 100 * time.Millisecond // 10 fps animation clock

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
	_ = term.SetMouseMode(terminal.MouseModeClick) // wheel + click select

	start, err := os.Getwd()
	if err != nil {
		start = "/"
	}
	app := newApp(term, start)
	if err := app.loadDir(start, ""); err != nil {
		term.Fini()
		fmt.Fprintln(os.Stderr, "read dir:", err)
		os.Exit(1)
	}

	// Animation clock: the input reader never emits {EventKey, KeyNone}
	// (unknown sequences are swallowed), so it is a collision-free
	// synthetic tick marker through the MPSC event channel.
	tickDone := make(chan struct{})
	go func() {
		t := time.NewTicker(tickInterval)
		defer t.Stop()
		for {
			select {
			case <-tickDone:
				return
			case <-t.C:
				term.PostEvent(terminal.Event{Type: terminal.EventKey, Key: terminal.KeyNone})
			}
		}
	}()

	for !app.quit {
		app.render()
		app.handleEvent(term.PollEvent())
	}
	close(tickDone)
	term.Fini()

	// --- cd-on-exit -----------------------------------------------------
	// A child cannot change the parent shell's cwd. Best scriptless
	// approximation: chdir + exec a fresh $SHELL (Q). Plain quit (q)
	// records the dir for optional shell-function integration.
	last := app.br.cwd
	_ = os.Chdir(last)
	if dir, err := os.UserCacheDir(); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "fv_lastdir"), []byte(last+"\n"), 0o600)
	}
	if app.execShell {
		sh := os.Getenv("SHELL")
		if sh == "" {
			sh = "/bin/sh"
		}
		if _, err := os.Stat(sh); err == nil {
			execShell(sh) // no return on success
		}
	}
	fmt.Fprintln(os.Stderr, last)
}
