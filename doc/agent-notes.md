# Agent notes

What earlier agents learned that neither `AGENTS.md` nor the docs state: traps and
techniques, a bullet each. Delete a note the code has since made impossible.

## Builds and workspaces

- A workspace that names `window/` in a `use` line and has no `replace` for the
  untagged root version breaks every build under it, the root's own too
  (`reading .../go.mod at revision vX.Y.Z: unknown revision`): Go still reads the
  `go.mod` of the version `window/go.mod` requires. In `window/`, `-mod=mod`
  rewrites `go.sum`.
- Go takes the `go.work` of any directory above the one it runs in, so a scratch
  module under one is refused (`directory prefix . does not contain modules
  listed in go.work`): `GOWORK=off`, or a `go.work` of its own. Its `go.mod`
  requires no untagged version, which would be fetched and fail.
- `go build` of one command leaves the binary in the working directory
  (`example-tui.exe` for `GOOS=windows`): build `./...`, or pass `-o /dev/null`.

## The browser build

- Without `env -i` the Node run of the js tests fails wherever the environment is
  large (`total length of command line and environment variables exceeds limit`):
  `wasm_exec.js` holds argv and the environment in 8 KB.
- The gate runs `-run Page`: a js test without Page in its name is not run by it.
- A fake `terminalPage` that calls a missing `event` from inside its own `post`
  hangs the Node test binary for minutes and fails nothing: check that `event` is
  a function first, as `pageWorker` does.
- A browser has no files: `wasm_exec.js`'s `fs` answers ENOSYS. `os.ReadDir` fails
  even over an `fs` answered from Go, since it opens with `O_DIRECTORY`, which
  `syscall` refuses while `fs.constants.O_DIRECTORY` is -1; `os.Open` and
  `f.ReadDir` work (`cmd/example-tui/files_js.go`).
- An untagged file shares its names with every platform's files and tests:
  `pagekeys.go` prefixes its own (`pageKey`, `pageMods`) because
  `keyconsole_test.go` already has a `keyEvent`.
- `node --check a.js b.js` checks `a.js` alone: one call a file.
- The page is driven headless with Playwright's Chromium. Wrapping `Worker`
  records the messages both ways; an `EventTarget` in place of
  `window.visualViewport` plays a soft keyboard; a `getContext('webgl')` that
  returns null gives the 2D painter, and a `MAX_TEXTURE_SIZE` of 256 a full mask.
- `cmd/example-tui` pulses its focused pane's border, so two screenshots of it
  differ by the phase: compare drawers on a program that stands still.

## The font bake

- Pillow has no switch for hinting, and Hack's hinting puts a horizontal line a
  row off from one glyph to the next: `┌─┐` jogs at 8x16 and 6x12 at any size and
  threshold. So the tool draws U+2500 to U+259F from a copy without instructions;
  drawing them as the rest breaks every box.
- A bake under another Pillow (FreeType) or Python (`unicodedata`) can flip a
  pixel or let through a rune `OneColumn` refuses. `atlas`'s test catches the
  rune and six lines; diff `dump` for the rest. The thinnest margin is 6x12's
  lines: thresholds 120 to 132 hold, 128 is used.
- Pillow rounds a fractional text position to a whole pixel and cuts a size to
  64ths of one: a line is moved in the font's units (a strike's `shift`), and the
  sizes are exact binary fractions.

## The window

- `TestAWindowOpensAtTheSizeItsManagerGivesIt` starts an Xvfb of its own and
  skips without `Xvfb`, `libEGL.so.1` or `libGLESv2.so.2` (Debian's `xvfb`,
  `libegl1`, `libgles2`, `libgl1-mesa-dri`): the package then reports `ok` with
  the window never opened. Read `-v`.
- That test is its own window manager. Xlib answers a second `XInternAtom` of
  one name from its cache, with no round trip: `XSync` is the round trip. An X
  error ends the process before a failure prints, so a window is kept until
  whatever still sizes it is done. Xvfb killed, not terminated, leaves its socket
  and lock behind.
- A program in the window is driven under `Xvfb` with `LIBGL_ALWAYS_SOFTWARE=1`:
  `xdotool key ctrl+minus` steps the zoom, `import -window root` takes the
  screen. A screenshot shows a cell's pixels and whether they are filtered, not
  which strike was stretched: the unit test reads that from the raster's `Bounds`.

## Tests

- A test over a host is deterministic with an unbuffered `Events` channel: a
  send returns once the Terminal is done with the event before it.
- A fixture of symmetric glyphs passes a mirrored drawer: draw one that is not.
- `android.DefaultCell` is 12 dp, so at density 1 a view draws with the 6x12
  strike: a test that wants 12x24 cells runs at density 2.
- Tried for `Font.index` and no faster: a search without a branch (the probes
  chain) and `sort.Find` (twice as slow, a closure a probe).
