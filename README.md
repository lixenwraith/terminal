# terminal

Direct ANSI terminal control for Go with zero-allocation rendering. Built for
sustained 60fps full-screen redraws in cell-based applications (games, dashboards,
TUIs). Depends only on the standard library and `golang.org/x/sys` (Unix builds).

The package bypasses terminfo/termcap entirely and emits ANSI sequences directly.
Target environments: xterm-compatible terminals on Linux and BSDs, and browsers
via xterm.js (WASM builds).

## Features

- True color (24-bit) and 256-color palette output with automatic capability detection
- Double-buffered output with cell-level diffing — only changed cells emit sequences
- Raw stdin parsing: keys, modifiers, UTF-8 runes, SGR mouse, resize
- Perceptual (Redmean) RGB → 256-palette mapping via O(1) LUT
- Color blending library: alpha, additive, screen, overlay, soft light
- Named color palettes for true color and xterm-256
- Panic-safe terminal restoration (`Fini`, `EmergencyReset`)
- Unix and WASM backends behind a common interface

## Architecture

    Terminal (interface)
      └── termImpl
            ├── outputBuffer   diffing, ANSI generation, 128KB buffered writer
            ├── inputReader    escape sequence parser, event channel
            └── Backend (interface)
                  ├── unixBackend   //go:build unix — termios, unix.Poll, SIGWINCH
                  └── wasmBackend   //go:build wasm — syscall/js, xterm.js bridge

Shared code carries no build tags: cell diffing, ANSI generation, escape parsing,
service lifecycle. Platform specifics are isolated in the `Backend` implementations.

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

import "github.com/lixenwraith/terminal"

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
            cells[i] = terminal.Cell{Rune: ' ', Bg: terminal.Gunmetal}
        }
        msg := "hello"
        for i, ch := range msg {
            // len(msg) to utf8.RuneCountIdString(msg) for non-ASCII
            cells[(h/2)*w+(w-len(msg))/2+i] = terminal.Cell{
                Rune: ch, Fg: terminal.Amber, Bg: terminal.Gunmetal,
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
    Fg    RGB
    Bg    RGB
    Attrs Attr
}
```

`Attr` is a bitmask: `AttrBold`, `AttrDim`, `AttrItalic`, `AttrUnderline`,
`AttrBlink`, `AttrReverse`.

Two flag bits change color interpretation: with `AttrFg256` / `AttrBg256` set,
`Fg.R` / `Bg.R` holds an xterm-256 palette index directly and `G`/`B` are
ignored. This allows exact palette output on true color terminals and skips
RGB → palette conversion.

## Color system

### Modes

`ColorModeTrueColor` emits `38;2;R;G;B` sequences; `ColorMode256` emits
`38;5;N` after mapping. `DetectColorMode()` inspects the environment
(`COLORTERM`, `TERM`). Explicit override: `terminal.New(terminal.ColorMode256)`.

### RGB → 256 mapping

`RGBTo256` maps any `RGB` to the nearest xterm-256 index using perceptually
weighted Redmean distance. The full mapping is pre-computed at init into a
6-bit-quantized LUT (256KB, L2-resident), making per-cell conversion a single
array load. Applications targeting 256-color terminals can render in RGB
throughout; degradation is automatic.

Palette helpers: `Cube256(r,g,b)` / `CubeRGB256(idx)` for 6×6×6 cube math,
`Gray256(step)` for the grayscale ramp, plus named constants (`P256Amber`,
`P256SteelBlue`, ...) in `rgb_256.go` and named true color values (`Amber`,
`Gunmetal`, `Obsidian`, ...) in `rgb_truecolor.go`.

### Blending

`blend.go` provides compositing primitives operating on `RGB`. All take
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
bg := terminal.Gunmetal
glow := terminal.RGB{R: 255, G: 160, B: 40}

cell.Bg = terminal.Screen(bg, terminal.Scale(glow, pulse), 1.0) // pulsing glow
cell.Bg = terminal.Blend(cell.Bg, terminal.Black, 0.6)          // dim overlay backdrop
cell.Fg = terminal.SoftLight(cell.Fg, tint, 0.4)                // subtle recolor
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
- `EventError`, `EventClosed`

A standalone ESC press is disambiguated from escape sequences by a short input-idle timeout (one ~10ms poll cycle).
Partial UTF-8 and escape sequences at read boundaries are reassembled in a persistent buffer.
`PostEvent` injects synthetic events (used for clean shutdown of blocked `PollEvent`).

## Service wrapper

`TerminalService` packages lifecycle (init, input goroutine, panic-safe
teardown) behind `Init/Start/Stop` for service-registry architectures:

```go
svc := terminal.NewService()
svc.Init()
svc.Start()
defer svc.Stop()

term := svc.Terminal()
for ev := range svc.Events() { /* ... */ }
```

Input-goroutine panics trigger `EmergencyReset` (restores cooked mode, main
screen, cursor) before printing the stack trace, keeping the shell usable.

## WASM

WASM builds bridge to xterm.js via JS globals:

    goTerminalWrite(Uint8Array)    // Go → JS terminal output
    goTerminalInput(Uint8Array)    // JS → Go keyboard input
    goTerminalResize(cols, rows)   // JS → Go resize
    xterm.cols, xterm.rows         // initial size query

## Sub-packages

- [`tui`](tui/README.md) — immediate-mode widget toolkit (regions, layout,
  widgets, scroll/editor state) built on the cell buffer model.
