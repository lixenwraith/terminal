# The page

What a program built on this module for js/wasm runs in. The Go half is
`page_js.go` in the root package; these are its scripts:

- `page.js` defines `TerminalPage`: it draws the program's frames on a canvas and
  reports keys, text, the pointer and its size back.
- `worker.js` is the program's thread, a Worker: it loads `wasm_exec.js`, runs the
  program and is the bridge between Go and the page.
- `index.html` and `page.css` are a minimal page that runs any program: a canvas
  filling the window, the hidden text field and, on a touch screen, a strip of keys.

## Build a page

    mkdir site && cp web/index.html web/page.css web/page.js web/worker.js site/
    GOOS=js GOARCH=wasm go build -o site/app.wasm ./cmd/example-tui
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" site/
    python3 -m http.server -d site

`wasm_exec.js` is the loader of the Go that built the binary: copy it at each
build. Any static server does, over HTTP: a `file:` page starts no Worker.
`index.html` runs `app.wasm` at once and shows a failure or the program's exit as
text. A browser has no files, so `cmd/example-tui` browses its own source there.

A program needs nothing for this: `terminal.New()` opens the page. `OpenPage` and
the `Page*` functions (the root README's Browser section) are for one that wants
its own font, zoom or messages. A page of one's own keeps `page.js` and
`worker.js` as they are and writes its script against what follows.

## TerminalPage.open(options)

Draws the screen at once, starts the download and returns the page object. A
document holds one page. `options`:

- `container`: the element the grid fills; the canvas is backed with its device
  pixels, and refitted whenever it resizes.
- `canvas`: the canvas in it, with no context yet. WebGL draws on it, a 2D context
  where there is none.
- `input`: a `textarea` in the container, focused but never seen: keys, composed
  text and pastes arrive through it. Style it as `page.css` does.
- `strip`: optional, the key strip, an element above the container. On a touch
  screen its `hidden` is cleared before the first fit. A `button` in it with
  `data-key` sends that key, a `KeyboardEvent.key` value, with `data-mods`
  (shift 1, alt 2, ctrl 4, meta 8), as a tap; its other buttons are the script's.
- `wasm`: the program's URL. `worker` and `exec`: `worker.js` and `wasm_exec.js`
  by default. Each is resolved against the document.
- `argv`: the program's `os.Args`, `['js']` by default.
- `kept`: `{name: text}`, what `PageKept(name)` returns, or a promise of it, which
  the start waits for and the download does not.
- `features`: the names `PageHas` answers true for: the program's own kinds this
  script handles.
- `hold`: true has the compiled program wait for `run()`.
- `cell`: a cell's height in CSS pixels, 16 by default; a cell is half as wide.
  The program's `PageConfig.Cell`, unless 0, replaces it.
- `font`: the CSS stack glyphs are drawn in, a list of monospace families by
  default. A program's own font (`PageConfig.Font`) goes ahead of it.
- `on`: the callbacks, each optional.

### Callbacks

None runs before `open` has returned.

- `progress(received, total)`: bytes of the program downloaded; `total` is 0 when
  the server did not say.
- `compiling()`: downloaded, and the worker starts.
- `ready()`: compiled; with `hold` it now waits for `run()`.
- `frame(first)`: a frame came, before it is drawn; `first` is true once.
- `message(msg)`: a message of the program's own, `PagePost(kind, fields)`, as
  `{type: kind, ...fields}`; a `PageBytes` field is a `Uint8Array`.
- `keep(name, text)`: `PageKeep`: a document to keep and hand back in `kept`.
- `gesture()`: a key press, or a pointer's press or release, of the player's.
- `keyboard(up)`: the soft keyboard was asked for or dropped, by `setKeyboard`,
  `toggleKeyboard`, three taps on the bar, or the system dismissing it.
- `notice(text)`: a sentence that ends nothing: no WebGL, a font that did not load.
- `exit(code, reason)`: the program exited and its worker is ended. `reason` is
  its crash line (`panic: ...`, `fatal error: ...`), else the last line it wrote to
  stderr, which may be empty.
- `failed(reason, stage)`: `reason` carries no final period. `stage` is
  - `wasm`: the browser has no WebAssembly;
  - `canvas`: the canvas gives no context;
  - `load`: the download, `wasm_exec.js` or the compile failed, or `kept`
    rejected; no worker is left;
  - `start`: the worker threw. The program may still run: `stop()` ends it.

### The page object

- `touch`: a touch-first screen (`pointer: coarse`), decided at load; also
  `TerminalPage.touch`, before `open`.
- `running`: a worker holds the program: from `compiling` until its exit, a
  `load` failure or `stop()`.
- `run()`: starts a held program.
- `send(event)`: an event to the program, as the page sends its own (below). A
  `type` terminal does not know goes to what `Page.Handle(type, fn)` registered.
- `stop()`: ends the program's worker; no callback follows.
- `focus()`: focuses the field, so keys reach the program.
- `lost()`: lets go of every button and finger held, as the page does when its
  window loses focus.
- `fit()`: fits the grid again. The page does on its own when the container or
  the pixel ratio changes, on a zoom, and on a fullscreen change on a touch screen.
- `setHidden(hidden)`: true holds the page as hidden whatever the document is, so
  `PageHidden()` is true; false goes back to the document's state, and the
  program's next frame is whole.
- `setKeyboard(up)`, `toggleKeyboard()`: raise or drop the soft keyboard on a
  touch screen, called inside the tap that asks; nothing elsewhere.

`TerminalPage.mods(event)` is a DOM event's modifiers as the program takes them.

### What the page does itself

- Fits the grid to the whole cells the container holds and reports each change.
- Keys: a key the program takes is sent with its `code` and its release. Left to
  the browser: chords with Meta or Alt, Ctrl-Shift, F5, F11, F12, Shift-Insert,
  and a Ctrl chord a terminal has no byte for. Composed text and a paste are one
  `text` each.
- Pointer: buttons, drags and the wheel at their cell while the program asked for
  the pointer, motion once a cell while it asked for motion. A finger is the left
  button, marked `touch`, and is sent with the pointer off too; Go then takes it
  on the bar's rows alone.
- Zoom: Ctrl and the wheel, a trackpad's pinch or two fingers move the cell within
  the program's range (4 to 40 in steps of 0.5 where it names none) and refit; the
  program gets a resize and, once the zoom holds still, the cell.
- Fingers: a second finger lets go of the first's press. Three taps on the bar
  (`PageConfig.BarRows`, at the grid's foot) toggle the soft keyboard.
- Soft keyboard: the grid keeps its size under it. The grid's last row is drawn
  again above the keyboard at twice its size, its left half filling the width, and
  the program learns the rows hidden (`Page.Covered`). This measures the visible
  height less the strip's, so the container must fill the window under the strip.
- Glyphs: each rune a frame brings is rasterized once at the cell's device size,
  from the font stack, into a mask of slots, and again on a zoom or a new font. A
  rune the mask has no room for (the GPU's texture limit, 4096 px a side at most)
  draws as U+FFFD.
- A hidden document: the program is told, and every key and button held is let go.

## The bridge

`worker.js` defines one global before the program runs, and Go defines none:

    globalThis.terminalPage = {
        grid: { cols, rows },   // the grid the page measured, kept current
        kept: { name: text },   // options.kept
        features: [ 'name' ],   // options.features
        hidden: false,          // the page is hidden
        frame(data),            // Go to page: a frame, a Uint8Array, transferred
        post(kind, fields),     // Go to page: { type: kind, ...fields }; typed arrays are transferred
        event: null,            // set by Go: function(msg), the page's messages
    };

Go to page, through `post`: `font` `{data}`, `zoom` `{cell, least, most, step,
bar}` (the first four each 0 for the page's own, `bar` the bar's rows), `pointer`
`{enabled, motion}`, `keep` `{name, text}`; any other kind is the program's own
and reaches `on.message`. The worker itself posts `ready`, `frame`, `exit` and
`failed`: a program names no kind so.

A frame is `terminal.RuneFrame`: little-endian 32-bit words `cols`, `rows`, `n`,
then `n` records of four: the cell's index, its rune, foreground and background as
`0xffRRGGBB`. Each frame is answered with `drawn`; Go sends the next after it.

Page to Go, through `event` (what `send` posts):

- `drawn` `{}`, `fitted` `{}` (the answer to `zoom`, once the font is loaded and
  the grid fits), `sync` `{}` (the next frame whole), `blur` `{}`
- `resize` `{cols, rows}`, `covered` `{rows}`, `zoom` `{cell}`
- `key` `{key, mods, code?, repeat?}`: with `code` a held key, paired with its
  `keyup` `{code}`; without, a tap
- `text` `{text}`: one `EventPaste`
- `mouse` `{x, y, button, action, mods, touch?}`: `button` 0 none, 1 left,
  2 middle, 3 right, 4 wheel up, 5 wheel down, 6 back, 7 forward; `action`
  1 press, 2 release, 3 move, 4 drag
- any other `type`: the program's own, for `Page.Handle`

Go reads the numbers listed without a default: an event that lacks one ends the
program.

The page talks to the worker in four messages: `start` `{exec, bytes, argv, cols,
rows, kept, features, hold}`, `run`, `event` `{event}` and `visibility` `{hidden}`.
