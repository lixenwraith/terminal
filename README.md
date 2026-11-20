# terminal

Direct ANSI terminal control for Go with zero-allocation rendering. Built for
sustained 60fps full-screen redraws in cell-based applications (games, dashboards,
TUIs). Depends on `github.com/lixenwraith/color` (RGB, palettes, blending),
`golang.org/x/sys` and `golang.org/x/term`; the desktop window, a module of its
own, alone adds `github.com/ebitengine/purego`.

The package bypasses terminfo/termcap entirely and emits ANSI sequences directly.
Target environments: xterm-compatible terminals on Linux, macOS and BSDs, Windows
consoles with VT processing, and three hosts that draw the cells themselves: a
browser page (js/wasm builds, on a canvas), a desktop window (X11, on Linux and
FreeBSD) and an Android view.

## Features

- True color (24-bit), 256-color, ANSI 16-color and colorless output with
  automatic capability detection, and the terminal's own default colors
- Double-buffered output with cell-level diffing — only changed cells emit sequences
- Raw stdin parsing, without allocation for common input: keys, modifiers, UTF-8
  runes, SGR mouse, resize, bracketed paste
- Draws on the controlling terminal when stdin or stdout is redirected (Unix)
- Perceptual (Redmean) RGB → 256-palette mapping via O(1) LUT, from `color`
- Panic-safe terminal restoration (`Fini`, `EmergencyReset`)
- Unix and Windows backends behind a common interface, and the same Terminal
  over a host that draws the cells itself: a browser page, a window, a phone's view
- A font for such a host: Hack as bitmaps at three cell sizes and as a web font

## Architecture

    Terminal (interface)
      ├── termImpl
      │     ├── outputBuffer   diffing, ANSI generation
      │     ├── writeLoop      the one goroutine that writes output
      │     ├── inputReader    escape sequence parser, event channel
      │     └── Backend (interface)
      │           ├── unixBackend     //go:build unix — termios, unix.Poll, SIGWINCH, /dev/tty
      │           └── windowsBackend  //go:build windows — console modes, VT processing, CONIN$/CONOUT$
      └── hostTerm             NewWithHost
            └── Host (interface)   a screen that draws the cells itself, and its input
                  ├── Page           //go:build js && wasm — OpenPage: a browser page, syscall/js
                  ├── android.View   an Android view; the app's drawer takes its frames
                  └── window.Window  its own module, linux || freebsd — Xlib, EGL, GLES 2

Shared code carries no build tags: cell diffing, ANSI generation, escape parsing.
Platform specifics are isolated in the `Backend` implementations and in the
hosts.

`NewWithBackend(b, mode)` builds a Terminal over a `Backend` of the caller's
own: a pipe, an in-process emulator or a test standing in for the tty. It
behaves as `New`'s Terminal does, the writer goroutine included; `New` is
`NewWithBackend` over the platform's backend and the detected color mode. Its
`Read` waits as long as the reader allows, without limit when nothing is
pending, and `Wake` ends that wait for a stop or a key report ask.

### Hosts

A `Surface` is what a finished frame is handed to (`ColorMode`, `Flush`, `Sync`):
a Terminal, or a host that draws the same cells itself, as a window, a browser
page or a phone's view does.

A `Host` is such a screen with its player's input: it also reports its grid
(`Size`), how long it has held a frame (`Blocked`) and its keys, pointer and
resizes as `Event`s on a channel, and takes a pointer mode and a key report ask.
`NewWithHost(h)` makes one a Terminal, so a program written to the interface runs
on it as it is. `SetMouseMode` becomes the pointer mode and `Clear` a frame of
blanks; the cursor calls do nothing, a paste is one `EventPaste` whatever the
paste mode, and `Fini` calls the host's `Close` if it has one.

`HostKeys` is the key report every host shares: presses until a program asks for
releases (`Ask`), then each key's press, repeats and release paired by the host's
own code for the key (`Down`, `Up`), a `Tap` for a key with no release to come,
and `Lost` as focus leaves. Each ask is answered with an `EventKeyReport`.

`Frames` is the `Surface` of a host whose drawer runs elsewhere, a view's thread
or a page: `Flush` keeps the newest frame, and `Take` encodes the cells changed
since the frame the drawer holds: nil when none did, every cell after a resize,
a `Sync` or a `SetEncoder`. `CellFrame` writes little-endian 32-bit words:
columns, rows and a count, then for each cell its index, an id for its rune, and
its colors as ARGB from `CellColors` (reverse, dim and a palette index resolved,
the terminal's own as light grey on black). `RuneFrame`, the default, has the
rune `Flush` would draw as the id, for a drawer that keeps glyphs of its own;
[`glyph`](glyph/README.md) has the encoders of one that paints from a bitmap face.

### Rendering pipeline

The application owns a flat `[]Cell` buffer (row-major, `cells[y*width+x]`) and
passes it to `Flush`. The output buffer diffs against the previously flushed frame:

- A row's changed cells come as a bitmap from `ChangedCells`: a block of 64
  cells, then of 16, that one memcmp finds unchanged costs that compare alone,
  and a cell compares as three words. The cells between the first and last set
  bits are drawn, a run of more than three unchanged ones moved over.
- Cursor moves are emitted only when the write position is non-contiguous.
- SGR state (fg, bg, attributes) is coalesced across cells: a cell in the style
  of the cell before it costs one compare, and redundant sequences are suppressed.
- A frame that writes is sent as one synchronized update (DEC mode 2026, 16
  bytes): a terminal that has the mode (Alacritty 0.13 and later, kitty,
  WezTerm, foot, Ghostty) draws the frame whole, never the rows a read cut it
  at; others ignore it.
- While the cursor is hidden, a frame that writes hides it again as it starts
  and ends (12 bytes), so a host that shows it on its own (a console host
  repainting, a resize) never shows it where the frame's last cell went. A
  cursor shown with `SetCursorVisible(true)` is left as it is.
- If the backend size changed between buffer preparation and `Flush`, the frame
  is dropped to prevent resize-race corruption. The next frame (built at the new
  size) renders normally.
- A cell is one glyph in one column, on a terminal that draws ambiguous-width
  runes (box drawing, U+FFFD) narrow: a control or format character in `Rune`
  (a pasted escape, a bidi override, a line separator) is drawn as a space,
  and a rune a terminal draws in no column, in two or in its neighbour's
  cluster (a combining or spacing mark, a wide or emoji rune) as U+FFFD.
  `OneColumn(r)` reports whether a cell draws `r` as itself, for a host that
  escapes or replaces the others its own way.

`Sync()` clears the screen and invalidates the front buffer, forcing a full
redraw — required after any external process writes to the terminal.

Output is written on a goroutine of its own, so no call waits on the terminal.
A terminal that stops reading (a hidden tab, a multiplexer client that went
away, flow control) holds that goroutine in its write; meanwhile `Flush` keeps
only the newest frame, drawn once the write returns, and control sequences
(cursor, mouse, paste, `Clear`, `Sync`) queue behind it in call order.
`Blocked()` reports how long the write in flight has waited, so a caller can
stop composing frames nobody will see. `Fini` waits up to two seconds for the
restoring sequences, then restores the termios without them.

Auto-wrap and, where the terminal follows ECMA-48 mode 8 (VTE), its own bidi
reordering are off during the session: the bottom-right cell is writable
without scroll side effects, and right-to-left text stays in its cells.

### Changed cells and SIMD

`ChangedCells(dst, a, b)` is the diff `Flush` runs on each row, exported for a
renderer that draws cells itself (a canvas, a texture, a framebuffer): bit
`i&63` of word `i>>6` is set where `a[i]` and `b[i]` differ, in `dst`'s storage.

Built with `GOEXPERIMENT=simd` (Go 1.27), it compares cells as vectors through
`simd/archsimd`: AVX2 on amd64 where the CPU has it, NEON on arm64, SIMD128 in
wasm, whose module then needs a browser with WebAssembly SIMD (every current
one). Other targets, and builds without the experiment, compare in plain Go;
the bits, and the bytes `Flush` writes, are the same either way.

The experiment is Go's and outside the Go 1 promise: a toolchain that changes
`simd/archsimd` can stop this build until the package follows, and on amd64 it
also moves the runtime's map hashing onto archsimd. Built without it, nothing
in the package changes.

## Quick start

```go
package main

import (
    "github.com/lixenwraith/color"
    "github.com/lixenwraith/terminal"
)

func main() {
    term := terminal.New() // color mode auto-detected
    if err := term.Init(); err != nil {
        panic(err)
    }
    defer term.Fini()

    w, h := term.Size()
    cells := make([]terminal.Cell, w*h)

    for {
        // Build frame
        for i := range cells {
            cells[i] = terminal.Cell{Rune: ' ', Bg: color.Gunmetal}
        }
        msg := "hello"
        for i, ch := range msg {
            // len(msg) to utf8.RuneCountIdString(msg) for non-ASCII
            cells[(h/2)*w+(w-len(msg))/2+i] = terminal.Cell{
                Rune: ch, Fg: color.Amber, Bg: color.Gunmetal,
                Attrs: terminal.AttrBold,
            }
        }
        term.Flush(cells, w, h)

        // Handle input
        ev := term.PollEvent()
        switch ev.Type {
        case terminal.EventKey:
            if ev.Key == terminal.KeyEscape || ev.Rune == 'q' {
                return
            }
        case terminal.EventResize:
            w, h = ev.Width, ev.Height
            cells = make([]terminal.Cell, w*h)
        }
    }
}
```

## Cells and attributes

```go
type Cell struct {
    Rune  rune
    Fg    color.RGB
    Bg    color.RGB
    Attrs Attr
}
```

`Attr` is a bitmask: `AttrBold`, `AttrDim`, `AttrItalic`, `AttrUnderline`,
`AttrBlink`, `AttrReverse`.

`Attr` is 16 bits wide; the bits above the styles say how to read a cell's
colors:
- With `AttrFg256` / `AttrBg256` set, `Fg.R` / `Bg.R` holds an xterm-256
  palette index directly and `G`/`B` are ignored. This allows exact palette
  output on true color terminals and skips RGB → palette conversion. Indices
  0-15 are the ANSI 16, which every terminal shows in its own theme's shades,
  and go out as `30-37`/`90-97` and `40-47`/`100-107`, 4 to 5 bytes shorter
  than `38;5;N`; a bold foreground below 8 keeps `38;5;N`, which a terminal
  drawing bold as bright may not brighten as it does `30-37`.
- With `AttrFgDefault` / `AttrBgDefault` set, the color is the terminal's own
  (SGR 39/49), whatever `Fg` / `Bg` hold: an application keeps a light or
  dark terminal's background without knowing which it is.

## Color system

### Modes

`ColorModeTrueColor` emits `38;2;R;G;B` sequences; `ColorMode256` emits
`38;5;N` after mapping; `ColorMode16` emits `30-37`/`90-97` and
`40-47`/`100-107`, an RGB or palette color mapped to the nearest ANSI index;
`ColorModeNone` emits attributes only, so a highlight drawn as a background
color alone (a list's `CursorBg`) does not show; the `tui` form controls
mark every selection with a glyph.

`DetectColorMode()` reports what the terminal can show from the environment
(`COLORTERM`, `TERM`): 16 colors on a text console (`TERM` of `linux`,
`cons25*`, `vt` and a digit, `ansi` or `*-16color`), whatever `COLORTERM`
says. In 16 colors every color change restates the style from a reset,
since a console's bright colors (90-97) also set bold. `New()`
also honours `NO_COLOR`, choosing `ColorModeNone`; a mode passed to `New`
overrides both, as a `--color=always` or `never` would. `Init` refuses
`TERM=dumb`, which cannot address the cursor.

On Windows the environment is read the same way where it names the terminal
(`TERM` or `COLORTERM` set, as mintty, ssh and Alacritty do). A native console
names none, and draws 24-bit color from Windows 10 1703 (build 15063) on, so
it gets true color; an older build gets 256. A console without VT processing
(conhost's legacy console) is refused at `Init`, which says so.

`Init` and `Sync` clear the screen to the terminal's own background.
`EmergencyReset(os.Stdout)` writes to the controlling terminal when stdout is
redirected, so the reset never lands in the program's output.

### RGB → 256 mapping

`RGBTo256` maps any `RGB` to the nearest xterm-256 index using perceptually
weighted Redmean distance. The full mapping is pre-computed at init into a
6-bit-quantized LUT (256KB, L2-resident), making per-cell conversion a single
array load. Applications targeting 256-color terminals can render in RGB
throughout; degradation is automatic.

Palette helpers: `Cube256(r,g,b)` / `CubeRGB256(idx)` for 6×6×6 cube math,
`Gray256(step)` for the grayscale ramp, plus named constants (`P256Amber`,
`P256SteelBlue`, ...) and named true color values (`Amber`, `Gunmetal`,
`Obsidian`, ...), all in `github.com/lixenwraith/color`.

### Blending

`github.com/lixenwraith/color` provides compositing primitives operating on `RGB`. All take
destination first and are branch-free in the hot path or LUT-backed; suitable
for per-cell use at frame rate.

- `Blend(dst, src, alpha)`: linear interpolation, standard transparency
- `Add(dst, src, alpha)`: saturating add, bright accumulation, clips
- `Screen(dst, src, alpha)`: `1-(1-d)(1-s)`, lightens, never clips
- `Overlay(dst, src, alpha)`: multiply/screen split at 0.5, contrast, keeps dst structure
- `SoftLight(dst, src, intensity)`: Perez soft light, gentle tint/glow
- `Max(dst, src, alpha)`: per-channel max, non-additive highlight
- `Scale(c, factor)`: channel multiply, dim/brighten
- `Grayscale(c)`: Rec. 601 luma, desaturation
- `c.Lerp(other, t)`: method on `RGB`, gradients, animation

`alpha`/`intensity`/`t` are `[0,1]`; out-of-range values clamp. `alpha` of 0 or 1
short-circuits without float math. All float→channel conversions round half-up,
so gradients from `Blend`, `Scale`, `Lerp`, and `SoftLight` are bit-consistent.

```go
bg := color.Gunmetal
glow := color.RGB{R: 255, G: 160, B: 40}

cell.Bg = color.Screen(bg, color.Scale(glow, pulse), 1.0) // pulsing glow
cell.Bg = color.Blend(cell.Bg, color.Black, 0.6)          // dim overlay backdrop
cell.Fg = color.SoftLight(cell.Fg, tint, 0.4)                // subtle recolor
bar := cold.Lerp(hot, load)                                     // value-mapped gradient
```

Integer paths (`Add`, `Screen`, `Overlay`) use a `(x + (x>>8) + 1) >> 8`
division approximation; `SoftLight` uses init-time LUTs replacing `math.Sqrt`.

## Input

`PollEvent()` blocks on a unified channel. `Event.Type` values:

- `EventKey` — `Key` for named keys (`KeyEnter`, `KeyUp`, `KeyCtrlC`, ...),
  `Key == KeyRune` with `Rune` set for printable input, `Modifiers` bitmask
  (`ModShift`, `ModAlt`, `ModCtrl`), `Action` (`KeyPress`, and `KeyRepeat` and
  `KeyRelease` while releases are reported; see below)
- `EventMouse` — 0-indexed `MouseX/Y`, `MouseBtn` (the three buttons, the
  wheel's four directions, back and forward), `MouseAction`
  (press/release/move/drag; a wheel step is a press with no release),
  modifiers. Enable via `SetMouseMode(MouseModeClick | MouseModeDrag)`; SGR
  protocol only.
- `EventResize` — new `Width`/`Height`
- `EventPaste` — `Text` holds a whole paste, as sent, control characters
  included. Enable via `SetPasteMode(true)` (bracketed paste); off, a paste
  arrives as keys, its newlines pressing Enter.
- `EventKeyReport` — the outcome of a `SetKeyReport` ask: `Report` and `Source`
- `EventFocus` — `Focused`, while releases come through the terminal's protocol
- `EventError`, `EventClosed`

A standalone ESC press is told from an escape sequence by input pausing 10 ms after it.
Otherwise the reader waits on input with no limit, so an idle terminal takes no wakeups: on Unix the read polls the terminal beside a pipe that `Wake` writes to, and only a lone ESC, a paste, a key protocol exchange or keys held on a console bound the wait. Windows still checks every 10 ms inside its read.
ESC ESC is Alt+Escape alone, Escape before a paste, and Alt with the key that follows otherwise (rxvt's Alt+arrows).
Partial UTF-8 and escape sequences at read boundaries are reassembled in a persistent buffer.
With paste mode off, a paste marker is not a paste. A paste whose end marker never arrives ends once input pauses for a second, so what follows a paste that stalls that long arrives as keys.

On Windows the console is read as VT input, so a mouse report arrives only if
the console host passes the request for reports on to the terminal: Windows
Terminal's does, and Windows 10's own ConPTY, which Alacritty uses unless a
`conpty.dll` and `OpenConsole.exe` from Windows Terminal lie beside it, is
reported not to.

A redirected stdin or stdout is replaced by the terminal itself, `/dev/tty`
on Unix and `CONIN$` or `CONOUT$` on Windows, so `app > out.toml` and
`cmd | app` keep both as data while the application draws and reads keys on
the terminal. Windows needs one of stdin, stdout and stderr left on the
console to know it is the one the player sees: with all three redirected
(mintty without ConPTY) the process's console is a hidden one, and `Init`
refuses it. A resize on Windows arrives with the console's size record, and a
poll every 100 ms catches a record a read took past.
`PostEvent` injects synthetic events (used for clean shutdown of blocked `PollEvent`).

### Key releases

A terminal reports presses: a key held down arrives as more presses at the
keyboard's repeat rate, and nothing says when it comes up. A game that needs
holds asks for releases:

```go
term.SetKeyReport(terminal.KeyReportReleases, terminal.KeySourceConsole|terminal.KeySourceProtocol)
```

The ask returns at once. An `EventKeyReport` follows with what the terminal
reports now (`Report`) and where releases come from (`Source`), so a game
learns the outcome of every ask and can fall back, to taps for one, where
there are none. `SetKeyReport(terminal.KeyReportPresses, 0)` gives the source
back. Until a game asks for releases, nothing is sent to the terminal or
switched, and every key event is a press, as before.

While releases are reported:

- Each key event carries its `Action`: `KeyPress`, `KeyRepeat` while held, and
  `KeyRelease`. A release carries what its press carried, paired by physical
  key, so letting go of Shift first still releases `'A'`, not `'a'`.
- A key event the terminal names no physical key for (text from an input
  method, a key its protocol leaves out) can't be held: its release follows
  at once.
- Every hold ends. Focus leaving, the console switching away, a change of
  source and `Fini` release the keys held with `Lost` set, since their own
  releases won't arrive. A press of a key already held means its release was
  lost (a terminal's own mode can swallow one): the old hold ends, `Lost`,
  before the new press.

Releases come from the first of these sources a game allows that the
terminal has:

- `KeySourceConsole`: the kernel console's own keyboard, on the console the
  game runs on: a Linux VT (`/dev/ttyN`) or FreeBSD's vt(4) (`/dev/ttyvN`).
  While releases are reported the package switches the keyboard to raw
  keycodes, which the kernel allows without root (Linux's `K_MEDIUMRAW` on
  the game's controlling tty, FreeBSD's `K_CODE` on a console the game has
  open), and switches it back when the game goes back to presses, at `Fini`
  and in `EmergencyReset`. Each key types what the console types for it
  cooked, by the console's own keymap read at the switch. Switching to
  another console ends the holds. The console's mouse reports pause while the
  keyboard is raw, and a mouse paste then reads as keys. Never over ssh,
  where the keyboard is the far terminal's.

  On Linux that is `loadkeys` layouts, dead keys and compose, the locks and
  their lights, Alt+F*n* and Alt+arrows switching consoles, and cursor keys
  with modifiers from Linux 7.1. Left out while raw: codes typed with Alt on
  the keypad, braille, Ctrl+Alt+Del, and Scroll Lock, which only lets stopped
  output go on.

  On FreeBSD it is `kbdmap` layouts and their accents, the locks and their
  lights, Alt+F*n* and Print Screen switching consoles, codes typed with left
  Alt on the keypad, and Alt as ESC before a character (right Alt too with
  `kern.vt.enable_altgr=0`). Left out while raw: the machine's keys
  (Ctrl+Alt+Del, the debugger, suspend), Shift+Insert's paste, and Scroll
  Lock. With Scroll Lock on, the console's history mode takes every key, so
  the keyboard stays as it is and the ask reports presses; an ask after
  Scroll Lock goes off tries again. syscons (`kern.vty=sc`) has no console
  source.

  Before the first switch the package runs the game's binary again as
  `console-guard`, which switches the keyboard back, restores the tty and
  shows the cursor if the game dies where nothing of its own runs: `kill -9`,
  the OOM killer, a Go fatal error. A game leading its console's session
  (on Linux, run with `exec` from the login shell or by an autologin) takes
  the console with it as it dies; the guard then opens it again to restore
  it. The package's `init` turns the guard's run aside before `main`; the
  `init`s of packages initialized before it run in the guard too, so keep
  side effects out of them. On FreeBSD the guard goes by the game's name, so
  `pkill -9` by that name ends it too: kill the game by its PID. Should the
  guard be killed as well, switch the keyboard back from elsewhere: on Linux
  `sudo kbd_mode -u -C /dev/ttyN`; FreeBSD has no command for it, so build
  one:

  ```sh
  printf '#include <sys/ioctl.h>\n#include <sys/kbio.h>\nint main(void) { return ioctl(0, KDSKBMODE, K_XLATE); }\n' |
      cc -x c -o /tmp/kbdxlate - && /tmp/kbdxlate < /dev/ttyvN
  ```

- `KeySourceProtocol`: the terminal's key event protocol, CSI u with event
  types. The package asks the terminal whether it has the protocol (`CSI ? u`,
  fenced by a DA1 request every terminal answers), pushes its flags and
  confirms what they came to before reporting releases; it goes by the
  terminal's answers, never by `TERM` or the terminal's name. Alacritty 0.13
  and later is the reference (Enter, Tab and Backspace release from 0.15).
  Other terminals that answer the same way (foot, Ghostty, WezTerm with its
  opt-in setting, recent Konsole) should work but are untested. Works over ssh.
  tmux, screen and mosh don't pass the protocol through, and VTE terminals and
  xterm don't have it; there the answer is presses after one round trip. The
  flags are popped when the game goes back to presses, at `Fini`, and in
  `EmergencyReset`.

  With the protocol on, every key arrives as an escape code, and each press
  reads as the key a legacy terminal sends for it reads, so a game can keep
  the protocol on for its whole run: Shift goes into the character, Ctrl with
  a character becomes its control key, and Alt stays a modifier. Ctrl with a
  character that has no control key (Ctrl+1) keeps `ModCtrl`, which a legacy
  terminal can't send. Escape no longer waits out the ESC timeout. Lone
  modifier keys, media keys and F13 and up report nothing. Alacritty reports no
  keys while its own Vi mode, search or hints run; a key held then ends on its
  next press, or as focus leaves.

`KeyHolds` is the pairing on its own, for a host that reads its platform's key
events and builds `Event`s itself (`KeySourceHost`): it gives each release its
press's key and turns the contradictions above into `Lost` releases. `HostKeys`
is such a host's whole report, over it.

A Terminal over a caller's backend (`NewWithBackend`) reports presses; one over a
host (`NewWithHost`) reports what its host does.

## Browser

Built for js/wasm, a program runs in a Worker of a web page, which draws its
cells on a canvas (WebGL, or 2D without it) and reports keys, text, the pointer
and its size as the events a terminal would. The page's scripts are in
[`web/`](web/README.md), with a minimal page any program runs in, how to build
one, and the script API a page of one's own is written against.

- `terminal.New()` returns a Terminal whose `Init` opens the page and whose color
  mode is true color whatever was asked; `Init` fails where there is no page, as
  under Node. `WindowSize` reports the page's grid.
- `OpenPage(PageConfig)` is for a program that wants more of its page: a web font
  handed over as bytes (`Font`; `atlas.HackWeb()` is one), the cell it opens at
  and the range a zoom keeps to (`Cell`, `CellLeast`, `CellMost`, `CellStep`),
  and the rows of a bar at the grid's foot that a finger still presses with the
  pointer off (`BarRows`). The `*Page` is a `Host`, for `NewWithHost`; it also
  reports the rows a soft keyboard hides (`Covered`) and the zoom the player left
  (`Cell`).
- A program's own messages go to its page's script with `PagePost(kind, fields)`
  (`PageBytes` for a byte slice) and come from it through `page.Handle(kind,
  fn)`. `PageHas` asks what that script handles, `PageKept` and `PageKeep` read
  and hand over a document the page keeps between visits, and `PageHidden` reports
  a page nobody sees.

## Sub-packages

- [`tui`](tui/README.md) — immediate-mode widget toolkit (regions, layout,
  widgets, scroll/editor state) built on the cell buffer model.
- [`inline`](inline/README.md) — styled text and in-place progress in the
  scrollback, for a CLI tool that does not own the screen.
- [`glyph`](glyph/README.md) — for a host that draws cells itself: a bitmap face
  at several cell sizes (its strikes), the software raster that draws a grid
  with one, and a strike as a texture with the frames encoded for it. Its README
  lays out the font file, the texture and the frames.
- `atlas` — Hack as such a face, at 6x12, 8x16 and 12x24, and as a web font. The
  application imports it and hands the face to its renderer, and no renderer
  imports it, so a binary embeds only the form it names:

  ```go
  // a window: github.com/lixenwraith/terminal/window
  win, err := window.Open(window.Config{Font: atlas.Hack(), Cols: 120, Rows: 40})
  term := terminal.NewWithHost(win) // in place of terminal.New()

  // a page, built for js/wasm
  page, err := terminal.OpenPage(terminal.PageConfig{Font: atlas.HackWeb()})
  term := terminal.NewWithHost(page)
  ```

  Hack comes under the MIT and Bitstream Vera licences:
  [`atlas/LICENSE`](atlas/LICENSE) travels with any binary that embeds the face.
  [`tool/font-atlas`](tool/font-atlas/README.md) regenerates both files.
- `android` — the Go half of an Android view, with no binding of its own. A
  `View` is a `Host`: the view's pixels are its grid, its keys, touches and text
  are `Event`s, and a pinch is a zoom, which refits the grid. The drawer is the
  application's, in Java or Kotlin: on its own thread it takes the strike's
  texture (`Atlas`, then `AtlasChanged` before each `Take`, which hands over the
  next strike's once a zoom crossed to it and the program drew the zoomed grid,
  so the drawer repaints once) and paints the frames from it, both as
  [`glyph/README.md`](glyph/README.md) lays them out. `Resize` comes first, before
  the program reads `Size` or a finger lands.
- `window` — a desktop window as a `Host`, on Linux and FreeBSD: Xlib opens it
  and reads its input, and EGL and GLES 2 show what `glyph.Raster` drew, each
  loaded at run time through purego, so the binary keeps `CGO_ENABLED=0`. Ctrl
  with `=`, `-` or the wheel zooms within `Config.Zoom`, and Ctrl with `0` goes
  back to the font's own size. It is a nested module,
  `github.com/lixenwraith/terminal/window`, tagged `window/vX.Y.Z`, so the root
  keeps its imports. A FreeBSD build passes
  `-gcflags=github.com/ebitengine/purego/internal/fakecgo=-std`.
