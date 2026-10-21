# terminal

Direct ANSI terminal control for Go with zero-allocation rendering. Built for
sustained 60fps full-screen redraws in cell-based applications (games, dashboards,
TUIs). Depends on `github.com/lixenwraith/color` (RGB, palettes, blending),
`golang.org/x/sys` and `golang.org/x/term`.

The package bypasses terminfo/termcap entirely and emits ANSI sequences directly.
Target environments: xterm-compatible terminals on Linux and BSDs, Windows
consoles with VT processing, and browsers via xterm.js (WASM builds).

## Features

- True color (24-bit), 256-color, ANSI 16-color and colorless output with
  automatic capability detection, and the terminal's own default colors
- Double-buffered output with cell-level diffing — only changed cells emit sequences
- Raw stdin parsing: keys, modifiers, UTF-8 runes, SGR mouse, resize, bracketed paste
- Draws on the controlling terminal when stdin or stdout is redirected (Unix)
- Perceptual (Redmean) RGB → 256-palette mapping via O(1) LUT, from `color`
- Panic-safe terminal restoration (`Fini`, `EmergencyReset`)
- Unix, Windows and WASM backends behind a common interface

## Architecture

    Terminal (interface)
      └── termImpl
            ├── outputBuffer   diffing, ANSI generation
            ├── writeLoop      the one goroutine that writes output
            ├── inputReader    escape sequence parser, event channel
            └── Backend (interface)
                  ├── unixBackend     //go:build unix — termios, unix.Poll, SIGWINCH, /dev/tty
                  ├── windowsBackend  //go:build windows — console modes, VT processing
                  └── wasmBackend     //go:build wasm — syscall/js, xterm.js bridge

Shared code carries no build tags: cell diffing, ANSI generation, escape parsing.
Platform specifics are isolated in the `Backend` implementations.

`NewWithBackend(b, mode)` builds a Terminal over a `Backend` of the caller's
own: a pipe, an in-process emulator or a test standing in for the tty. It
behaves as `New`'s Terminal does, the writer goroutine included; `New` is
`NewWithBackend` over the platform's backend and the detected color mode.

### Rendering pipeline

The application owns a flat `[]Cell` buffer (row-major, `cells[y*width+x]`) and
passes it to `Flush`. The output buffer diffs against the previously flushed frame:

- Rows are scanned with early termination (trailing unchanged cells skipped).
- Cursor moves are emitted only when the write position is non-contiguous.
- SGR state (fg, bg, attributes) is coalesced across cells; redundant sequences
  are suppressed.
- If the backend size changed between buffer preparation and `Flush`, the frame
  is dropped to prevent resize-race corruption. The next frame (built at the new
  size) renders normally.
- A cell is one glyph in one column, on a terminal that draws ambiguous-width
  runes (box drawing, U+FFFD) narrow: a control or format character in `Rune`
  (a pasted escape, a bidi override, a line separator) is drawn as a space,
  and a rune a terminal draws in no column, in two or in its neighbour's
  cluster (a combining or spacing mark, a wide or emoji rune) as U+FFFD.

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
  0-15 are the ANSI 16, which every terminal shows in its own theme's shades.
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
- `EventMouse` — 0-indexed `MouseX/Y`, `MouseBtn` (buttons, wheel),
  `MouseAction` (press/release/move/drag), modifiers. Enable via
  `SetMouseMode(MouseModeClick | MouseModeDrag)`; SGR protocol only.
- `EventResize` — new `Width`/`Height`
- `EventPaste` — `Text` holds a whole paste, as sent, control characters
  included. Enable via `SetPasteMode(true)` (bracketed paste); off, a paste
  arrives as keys, its newlines pressing Enter.
- `EventKeyReport` — the outcome of a `SetKeyReport` ask: `Report` and `Source`
- `EventFocus` — `Focused`, while releases come through the terminal's protocol
- `EventError`, `EventClosed`

A standalone ESC press is disambiguated from escape sequences by a short input-idle timeout (one ~10ms poll cycle).
ESC ESC is Alt+Escape alone, Escape before a paste, and Alt with the key that follows otherwise (rxvt's Alt+arrows).
Partial UTF-8 and escape sequences at read boundaries are reassembled in a persistent buffer.
With paste mode off, a paste marker is not a paste. A paste whose end marker never arrives ends once input pauses for a second, so what follows a paste that stalls that long arrives as keys.

On Unix, a redirected stdin or stdout is replaced by `/dev/tty`, so
`app > out.toml` and `cmd | app` keep both as data while the application
draws and reads keys on the terminal.
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
press's key and turns the contradictions above into `Lost` releases.

A Terminal over a caller's backend (`NewWithBackend`) reports presses.

## WASM

WASM builds bridge to xterm.js via JS globals:

    goTerminalWrite(Uint8Array)    // Go → JS terminal output
    goTerminalInput(Uint8Array)    // JS → Go keyboard input
    goTerminalResize(cols, rows)   // JS → Go resize
    xterm.cols, xterm.rows         // initial size query

## Sub-packages

- [`tui`](tui/README.md) — immediate-mode widget toolkit (regions, layout,
  widgets, scroll/editor state) built on the cell buffer model.
