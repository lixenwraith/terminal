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
            ├── outputBuffer   diffing, ANSI generation, 128KB buffered writer
            ├── inputReader    escape sequence parser, event channel
            └── Backend (interface)
                  ├── unixBackend     //go:build unix — termios, unix.Poll, SIGWINCH, /dev/tty
                  ├── windowsBackend  //go:build windows — console modes, VT processing
                  └── wasmBackend     //go:build wasm — syscall/js, xterm.js bridge

Shared code carries no build tags: cell diffing, ANSI generation, escape parsing.
Platform specifics are isolated in the `Backend` implementations.

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

`Sync()` clears the screen and invalidates the front buffer, forcing a full
redraw — required after any external process writes to the terminal.

Auto-wrap is disabled during the session, making the bottom-right cell writable
without scroll side effects.

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

| Function | Operation | Character |
|---|---|---|
| `Blend(dst, src, alpha)` | linear interpolation | standard transparency |
| `Add(dst, src, alpha)` | saturating add | bright accumulation, clips |
| `Screen(dst, src, alpha)` | `1-(1-d)(1-s)` | lightens, never clips |
| `Overlay(dst, src, alpha)` | multiply/screen split at 0.5 | contrast, keeps dst structure |
| `SoftLight(dst, src, intensity)` | Perez soft light | gentle tint/glow |
| `Max(dst, src, alpha)` | per-channel max | non-additive highlight |
| `Scale(c, factor)` | channel multiply | dim/brighten |
| `Grayscale(c)` | Rec. 601 luma | desaturation |
| `c.Lerp(other, t)` | method on `RGB` | gradients, animation |

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
  (`ModShift`, `ModAlt`, `ModCtrl`)
- `EventMouse` — 0-indexed `MouseX/Y`, `MouseBtn` (buttons, wheel),
  `MouseAction` (press/release/move/drag), modifiers. Enable via
  `SetMouseMode(MouseModeClick | MouseModeDrag)`; SGR protocol only.
- `EventResize` — new `Width`/`Height`
- `EventPaste` — `Text` holds a whole paste, as sent, control characters
  included. Enable via `SetPasteMode(true)` (bracketed paste); off, a paste
  arrives as keys, its newlines pressing Enter.
- `EventError`, `EventClosed`

A standalone ESC press is disambiguated from escape sequences by a short input-idle timeout (one ~10ms poll cycle).
ESC ESC is Alt+Escape alone, Escape before a paste, and Alt with the key that follows otherwise (rxvt's Alt+arrows).
Partial UTF-8 and escape sequences at read boundaries are reassembled in a persistent buffer.
A cell is drawn as one glyph: a control character in `Rune`, such as a pasted escape, is drawn as a space.
With paste mode off, a paste marker is not a paste. A paste whose end marker never arrives ends once input pauses for a second, so what follows a paste that stalls that long arrives as keys.

On Unix, a redirected stdin or stdout is replaced by `/dev/tty`, so
`app > out.toml` and `cmd | app` keep both as data while the application
draws and reads keys on the terminal.
`PostEvent` injects synthetic events (used for clean shutdown of blocked `PollEvent`).

## WASM

WASM builds bridge to xterm.js via JS globals:

    goTerminalWrite(Uint8Array)    // Go → JS terminal output
    goTerminalInput(Uint8Array)    // JS → Go keyboard input
    goTerminalResize(cols, rows)   // JS → Go resize
    xterm.cols, xterm.rows         // initial size query

## Sub-packages

- [`tui`](tui/README.md) — immediate-mode widget toolkit (regions, layout,
  widgets, scroll/editor state) built on the cell buffer model.
