# TODO

Each item states what is owed and the smallest thing that would close it. Delete
an item when it lands; this file is not a changelog.

Priorities: P1 is wanted next, P2 is convenient follow-up, P3 is an idea.

## Run each host on the real thing

- Priority: P1
- Affected files: `web/page.js`, `page_js.go`, `window/`, `android/`

Each host that draws was run in simulation only:

- The page: headless Chromium. Not run: Firefox, Safari, iOS (the 4096 px a
  canvas holds, which `MASK_MOST` rests on, is from memory), a real soft
  keyboard, a real pinch, an input method's composition, a GPU that loses its
  context, holds textures under 4096 px or has no `highp` in a fragment shader.
- The window on X11: Xvfb with Mesa's software GL, through GLES 2, and through
  desktop OpenGL in a Steam Linux Runtime 4 tree. Not run: a GPU, a vendor's
  `libOpenGL.so.0`, a window manager (`Maximized`, `Fullscreen`), FreeBSD, a
  `Scale` above 1 outside its unit test.
- The window on Windows: Wine 9.0 under Xvfb. Not run: Windows itself, arm64, a
  DPI change between monitors, an input method, dead keys and left Ctrl+Alt as
  Windows delivers them, a border dragged under Windows' own size loop, a touch
  screen, a high-refresh display.
- The Android view: its tests. A strike swap under a drawer's own thread needs an
  app on a device.

Closes with one run of `cmd/example-tui`'s page in each browser and on a phone,
and of a program in the window on a desktop, on FreeBSD and on Windows 10 and
11 (`go test -v ./...` in `window\` first), zoomed across the strikes; what a
run finds becomes its own item.

## The Windows window's blit is not measured on Windows

- Priority: P1
- Affected files: `window/window_windows.go` (`draw`, `recell`)

Its present is the raster, which is Go's and measured (0.4 to 2.7 ms a frame at
1908x1080 with 18x36 cells, 1.7 to 11.2 ms at 3834x2160), and one
`StretchDIBits`, measured under Wine alone (about 4.5 ms at 1920x1080 and 20 ms
at 3840x2160 for the 1:1 copy). Closes with `go test -run '^$' -bench Present
-benchtime 100x .` run on Windows: if the x2 stretch is slower there than the
copy by more than the raster it saves, `recell` sends whole multiples through
the scaled raster too; if the copy itself is slow, a GPU present (Direct3D 11
or WGL).

## U+FFFD has no glyph in the face

- Priority: P2
- Affected files: `tool/font-atlas/font-atlas.py`, `atlas/`, `glyph/raster.go`,
  `glyph/atlas.go`, `output.go` (`oneColumn`)

Hack has no U+FFFD, so `atlas.Hack()` has none, and `glyph` looks a cell's rune
up as it is: a rune `Flush` draws as U+FFFD (a wide or emoji rune, a combining
mark) is a blank cell on a `Raster` and has no slot in a `Texture`. A page is
not affected: `RuneFrame` sends U+FFFD and its fallback fonts draw it. Closes
with a glyph for U+FFFD made by the tool in each strike, and the raster and the
texture drawing the rune `Flush` would: `OneColumn` says only whether a cell
draws a rune as itself, and the rune drawn instead, a space or U+FFFD, is
`oneColumn`'s, unexported.

## The raster has two painters

- Priority: P2
- Affected files: `glyph/raster.go` (`draw`, `blend`)

A cell of the strike's own size is painted by the older loop, a bit test and
four byte stores a pixel; a cell of any other size by coverage masks and a table
of mixes, which is the faster at the same pixels: a 1920x1080 frame of 12x24
cells takes 0.83 and 5.5 ms (14 % and all cells changed) through `draw` and 0.51
and 3.5 ms through `blend`. Closes with every size on `blend` and `draw`
deleted, which speeds the X11 window and Android's raster as well.

## The X11 window has no paste

- Priority: P2
- Affected files: `window/window_x11.go`

The Windows window posts the clipboard as an `EventPaste` on Ctrl-Shift-V and
Shift-Insert; on X11 both reach the program as keys. Closes with the CLIPBOARD
selection read as UTF8_STRING (`XConvertSelection`, `SelectionNotify`, INCR for
a large one) on the same chords.

## A host draws no cursor

- Priority: P2
- Affected files: `host.go`, `page_js.go`, `web/page.js`, `android/view.go`,
  `window/window.go`

Over `NewWithHost`, `SetCursorVisible` and `MoveCursor` do nothing, so a program
that shows the terminal's own cursor has none on a page, in a window or on a
view. `tui` draws its cursor as a cell and is not affected. Closes with a method
a host may have, as it may have `Close`, that `hostTerm` hands both calls to,
and one host drawing the cursor from it.

## `New`'s Terminal in a browser before `Init`

- Priority: P2
- Affected files: `page_js.go` (`pageTerm`)

On js/wasm `New` returns a Terminal with no page under it until `Init` opens
one: any method but `Init` and `Fini` called before that, or after an `Init`
that failed, is a nil dereference, where a tty's Terminal answers `Size` and
`ColorMode` and ignores a draw. Closes with `pageTerm` answering those before
`Init` (`WindowSize`, truecolor, nothing drawn).

## Neovim's terminal confirms key flags it does not act on

- Priority: P3
- Affected files: `keyprotocol.go`

Neovim's terminal (0.11.7, 0.12.6) answers `CSI ? u` with every flag pushed and
acts on `disambiguate` alone, so `protoNeed` is met and a program in it is told
`KeySourceProtocol` with releases where every key taps. Closes with Neovim
answering the flags it implements, reported upstream; failing that, the
protocol declined where `$NVIM` is set, which goes by the environment where the
module goes by answers, and costs a terminal started from Neovim its releases.

## The X11 window's keypad types nothing

- Priority: P3
- Affected files: `window/keys.go` (`keysymRune`)

KP_0 to KP_9, KP_Add, KP_Subtract, KP_Multiply, KP_Divide and KP_Decimal are
neither in `keysymKeys` nor Latin-1, so with Num Lock on the keypad posts no
event, where a terminal, and the Windows window, report the character. Closes
with those keysyms mapped to their characters.

## The Windows window's icon is the system's

- Priority: P3
- Affected files: `window/window_windows.go` (`register`), `window/window.go`
  (`Config`)

The class has no icon, so the title bar, the taskbar and Alt+Tab show the
default one whatever icon the executable carries. Closes with the executable's
first icon resource loaded for the class, or a `Config` field.

## A button held when the Windows window loses capture is never released

- Priority: P3
- Affected files: `window/window_windows.go` (`mouse`)

Alt+Tab during a drag takes the capture, and the button's release goes to
another window: the program sees a press with no release. Closes with
`WM_CAPTURECHANGED` posting the releases of the buttons still down.

## The Windows window's mouse and sizing have no test

- Priority: P3
- Affected files: `window/window_windows_test.go`

Its key messages have one (`TestAKeyMessageReportsWhatATerminalReports`); its
buttons, wheel steps and `WM_SIZING` snap were checked by hand under Wine.
Closes with the same kind of test: messages handed to `handle`, the events read
back.

## One frame in flight has no test

- Priority: P3
- Affected files: `page_js.go` (`present`), `page_js_test.go`

A `Page` hands the page one frame and the next only once `drawn` answers it; a
simulated run showed it and no test holds it. Closes with the fake worker of
`page_js_test.go` failing on a second frame before its `drawn`.

## The page's mask upload is not measured

- Priority: P3
- Affected files: `web/page.js` (`Drawer.rasterize`)

A frame that brings a rune the page has not drawn uploads the whole glyph mask
again, up to 4096 px a side. Closes with the upload timed on a frame of new
runes at the largest mask; if it shows, `texSubImage2D` of the rows that gained
slots.

## Benchmark `glyph` on Hack

- Priority: P3
- Affected files: `atlas/atlas_test.go`, `glyph/atlas.go` (`Texture.Slot`)

`BenchmarkRasterFlush` draws a packed fixture of 300 glyphs: a test in package
`glyph` cannot import `atlas`, which imports it. A glyph is found by binary
search, which over Hack's 1,538 runes takes 45 to 75 ns for a rune out of order
where the map it replaced took 7; a whole cell frame on a phone is not
measured. Closes with a benchmark of a whole `Texture.Frame` on `atlas.Hack()`
in `atlas`'s tests, and a map in `Texture` if the search shows there.

## `EmergencyReset` in a browser writes escape sequences

- Priority: P3
- Affected files: `terminal.go` (`EmergencyReset`)

Built for js/wasm, `EmergencyReset(os.Stdout)` still writes its sequences, which
`wasm_exec.js` prints to the console as text. Closes with it doing nothing there.

## `cmd/example-keys` asks no host for releases

- Priority: P3
- Affected files: `cmd/example-keys/main.go`

Its ask for releases, at start and on F2, names
`KeySourceConsole|KeySourceProtocol`, so built for js/wasm its page answers
presses. Closes with `KeySourceHost` in the ask and a browser page in its
header's list.

## `inline`'s `SetColor` races `Paint`

- Priority: P2
- Affected files: `inline/inline.go` (`Printer.color`), `inline/style.go`
  (`Paint`)

`SetColor` writes the setting under the printer's lock and `Paint` reads it
without, so the two on different goroutines race (`go run -race` reports it);
the README and the `Printer` doc say so. Closes with `color` an `atomic.Bool`
and that exception dropped from both.

## Nerd Fonts icon sets

- Priority: P3, after the owner's review
- Affected files: `tool/font-atlas/font-atlas.py`, `atlas/`, `glyph/atlas.go`
  (`Texture`)

The face is Hack's own glyphs, its seven Powerline ones among them; no Nerd Fonts
icon set is baked. Three things stand in the way: each set comes under a license
of its own, which the owner has not reviewed; the icons read poorly as one-bit
glyphs in a 12x24 cell, and worse below; and a set is thousands of glyphs, where
a strike's texture holds a slot for every glyph of its font, so a bitmap host
would need slots made as frames bring runes, as the page's mask makes them.
Closes with the review, then one set baked as a font file of its own and judged
from `dump`, and slots on demand only if it stays.
