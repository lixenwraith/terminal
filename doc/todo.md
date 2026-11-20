# TODO

Each item states what is owed and the smallest thing that would close it. Delete
an item when it lands; this file is not a changelog.

Priorities: P1 is wanted next, P2 is convenient follow-up, P3 is an idea.

## Complete `window/go.sum` once the root version is tagged

- Priority: P1
- Affected files: `window/go.sum`

`window/go.sum` holds purego's and `x/sys`'s lines alone. The root version its
`go.mod` requires has no tag yet, so its sum cannot be fetched, and `color`'s and
`x/term`'s are missing with it: under a workspace nothing reads them, and with
`GOWORK=off` the module does not build (`missing go.sum entry`). Closes with
`GOWORK=off go mod tidy` in `window/` once the tag resolves, and the window's
gates run there without a workspace.

## Run each host on the real thing

- Priority: P1
- Affected files: `web/page.js`, `page_js.go`, `window/`, `android/`

Each host that draws was run in simulation only:

- The page: headless Chromium. Not run: Firefox, Safari, iOS (the 4096 px a
  canvas holds, which `MASK_MOST` rests on, is from memory), a real soft
  keyboard, a real pinch, an input method's composition, a GPU that loses its
  context, holds textures under 4096 px or has no `highp` in a fragment shader.
- The window: Xvfb with Mesa's software GL. Not run: a GPU, a window manager
  (`Maximized`, `Fullscreen`), FreeBSD, a `Scale` above 1 outside its unit test.
- The Android view: its tests. A strike swap under a drawer's own thread needs an
  app on a device.

Closes with one run of `cmd/example-tui`'s page in each browser and on a phone,
and of a program in the window on a desktop and on FreeBSD, zoomed across the
strikes; what a run finds becomes its own item.

## U+FFFD has no glyph in the face

- Priority: P2
- Affected files: `tool/font-atlas/font-atlas.py`, `atlas/`, `glyph/raster.go`,
  `glyph/atlas.go`, `output.go` (`oneColumn`)

Hack has no U+FFFD, so `atlas.Hack()` has none, and `glyph` looks a cell's rune
up as it is: a rune `Flush` draws as U+FFFD (a wide or emoji rune, a combining
mark) is a blank cell on a `Raster` and has no slot in a `Texture`. A page is
not affected: `RuneFrame` sends U+FFFD and its fallback fonts draw it. Closes
with a glyph for U+FFFD made by the tool in each strike, and the raster and the
texture drawing the rune `Flush` would (`oneColumn`, unexported today).

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

## Nerd Fonts icon sets

- Priority: P3, after the owner's review
- Affected files: `tool/font-atlas/font-atlas.py`, `atlas/`, `glyph/atlas.go`
  (`Texture`)

The face is Hack's own glyphs, its seven Powerline ones among them; no Nerd Fonts
icon set is baked. Three things stand in the way: each set comes under a licence
of its own, which the owner has not reviewed; the icons read poorly as one-bit
glyphs in a 12x24 cell, and worse below; and a set is thousands of glyphs, where
a strike's texture holds a slot for every glyph of its font, so a bitmap host
would need slots made as frames bring runes, as the page's mask makes them.
Closes with the review, then one set baked as a font file of its own and judged
from `dump`, and slots on demand only if it stays.
