// fv — file voyager: an lf / midnight-commander / xxd hybrid.
// Demo application for the terminal + tui + color packages.
//
// Layout:   [parent dir] [current dir list] [preview: info + text/hex/dir table]
// Enter on a file opens a fullscreen hex viewer with byte cursor and search.
//
// Keys (browse): j/k/arrows move · h/l or ←/→ parent/enter · Enter open
//
//	gg/G top/bottom · PgUp/PgDn, Ctrl+D/U page · ~ home · . hidden · s sort
//	/ live filter (Esc clears) · Space mark · a mark-all · u unmark
//	y yank paths · r rename · m mkdir · D delete (confirm) · T theme
//	? help · Ctrl+L redraw · q quit · Q quit + exec $SHELL in last dir
//
// Keys (hex):    hjkl/arrows · 0/$ row ends · g/G · PgUp/PgDn · Ctrl+D/U
//
//	/ search (hex bytes like "de ad" / "0xdead", or literal text) · n/N cycle
//	Esc/q back
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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
			_ = syscall.Exec(sh, []string{sh}, os.Environ()) // no return on success
		}
	}
	fmt.Fprintln(os.Stderr, last)
}
