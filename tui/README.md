# tui

Immediate-mode widget toolkit on top of the `terminal` cell buffer. No retained
widget tree, no framework loop: the application owns a `[]terminal.Cell` buffer
and all state; `tui` provides regions, layout math, render functions, and
plain-struct state helpers. Every frame is a full logical redraw — the
`terminal` diff layer keeps actual output minimal.

## Core concept: Region

A `Region` is a bounds-checked rectangular view over a cell slice. All drawing
goes through regions; coordinates are region-relative. Sub-regions nest and
clip to parent bounds, so widgets cannot draw outside their allotted area.

```go
w, h := term.Size()
cells := make([]terminal.Cell, w*h)
root := tui.NewRegion(cells, w, 0, 0, w, h)

panel := root.Sub(2, 1, 40, 10) // clipped view
inner := panel.Inset(1)         // shrink 1 cell on all sides
```

Out-of-bounds writes are silently dropped — no bounds management needed in
widget code.

## Quick start

```go
term := terminal.New()
term.Init()
defer term.Fini()

w, h := term.Size()
cells := make([]terminal.Cell, w*h)

list := tui.NewScrollState(len(items), h-2)
list.Selection = 0

for {
    root := tui.NewRegion(cells, w, 0, 0, w, h)
    root.Fill(color.Gunmetal)

    root.Box(tui.LineRounded, color.SteelBlue)
    content := root.Inset(1)
    content.List(buildItems(items), list.Selection, list.Offset, tui.ListOpts{
        CursorBg: color.DarkSlate,
    })
    content.ScrollBar(content.W-1, list.Offset, list.Visible, list.Total,
        color.IronGray)

    term.Flush(cells, w, h)

    ev := term.PollEvent()
    switch ev.Type {
    case terminal.EventKey:
        switch ev.Key {
        case terminal.KeyUp:
            list.SelectPrev()
        case terminal.KeyDown:
            list.SelectNext()
        case terminal.KeyEscape:
            return
        }
    case terminal.EventResize:
        w, h = ev.Width, ev.Height
        cells = make([]terminal.Cell, w*h)
        list.SetVisible(h - 2)
    }
}
```

## Layout

```go
cols := tui.SplitH(root, 0.3, 0.7)          // ratio split, normalized
rows := tui.SplitV(cols[1], 0.5, 0.5)
side, main := tui.SplitHFixed(root, 24)     // fixed left width
top, rest := tui.SplitVFixed(root, 3)       // fixed top height
dlg := tui.Center(root, 50, 12)             // centered sub-region
```

The last ratio segment absorbs rounding remainder — no gaps.

## Text and style

```go
r.Text(x, y, "label", fg, bg, terminal.AttrNone)
r.TextCenter(y, "title", fg, bg, terminal.AttrBold)
r.TextRight(y, "hint", fg, bg, terminal.AttrDim)
lines := r.TextBlock(x, y, longText, fg, bg, attr) // word-wrapped, returns line count
r.TextStyled(x, y, s, tui.Style{Fg: fg, Bg: bg, Attr: attr})
```

String utilities operate on rune counts: `RuneLen`, `Truncate` /
`TruncateLeft` / `TruncateMiddle` (ellipsis variants), `PadLeft` / `PadRight` /
`PadCenter`, `WrapText` (at line breaks and spaces; a path or key longer than
the line breaks after `.`, `,`, `:`, `/`, `[` or `]`).

`Style{Fg, Bg, Attr}` bundles cell appearance; most widget option structs
accept it.

## Widgets

Widgets are stateless render functions (mostly `Region` methods). Application
state lives in plain structs passed by pointer. Available renderers:

boxes and lines (`Box`, `BoxFilled`, `HLine`, `VLine` — single, double,
rounded, heavy, dashed and ASCII line types), `List`, `Table`, `Tree`, `TabBar`, `KeyValue` /
`KeyValueWrap`, `Progress` / `ProgressV` / `Gauge` / `Spinner`,
`ProgressOverlay`, `Sparkline` / `SparklineV`, `Input` / `TextField`,
`Editor`, `Modal` / `Overlay` / `ConfirmDialog`, `ScrollBar` /
`ScrollIndicator`, masonry layout.

Representative patterns below; remaining widgets follow the same
opts-struct + state-struct shape — read the source for full options.

### Theme and form controls

A `Theme` is one `Style` per role (`Text`, `Muted`, `Accent`, `Selected`,
`Input`, `Cursor`, `Error`, `Border`) plus the `Glyphs` widgets draw. A
role's `Attr` may carry color bits, a palette index or the terminal's own
color, so an application builds one theme per color tier and every control
draws alike in each: `DefaultTheme` is dark true color, `Theme16` uses the
ANSI 16 on the terminal's own background, `MonoTheme` uses attributes
alone, and `ThemeFor(term.ColorMode())` picks among them. A selection
always changes a glyph (a radio mark, a pointer, the focus mark), so it reads
without color. `GlyphsUnicode`, `GlyphsCP437` (text consoles) and
`GlyphsASCII` (outside UTF-8) are the glyph sets; `Glyphs.Line` is the line
type of frames, rules and wires.

```go
th := tui.DefaultTheme
root.FillStyle(th.Text) // the panel the controls draw on

value, rows := form.Field(y, 10, tui.Field{Label: "port", Required: true,
    Help: "TCP port to listen on", Error: portErr}, focus.Index == 0, th)
value.TextInput(port, "8080", focus.Index == 0, th) // the default as placeholder
y += rows

value, rows = form.Field(y, 10, tui.Field{Label: "format"}, focus.Index == 1, th)
value.Radio(0, 0, []string{"raw", "txt", "json"}, format, th) // ○ raw  ● txt  ○ json
y += rows

form.Group(y, "tls", "on, pinned", tlsOpen, focus.Index == 2, th)
```

- `Focus` is the focus ring: `HandleKey` moves it on Tab and Shift+Tab.
- `Field` draws the focus mark, the label, the required mark, and the error,
  or the help while focused, wrapped, and returns the region for the value.
  The label leaves the value at least half the row; label width 0 stacks the
  label above the value, for narrow screens.
- `TextInput` draws a `TextFieldState`, its placeholder muted while empty,
  scrolled to the cursor with a muted `…` where text is out of view.
  `TextFieldState.Accept` filters typed and pasted runes (`AcceptInteger`,
  `AcceptNumber`); `Paste` inserts an `EventPaste`'s text with line breaks
  as spaces and control characters dropped.
- `Radio` draws radio buttons in a row, the chosen one marked; too wide, it
  shows the chosen one and its neighbours. `StepChoice` moves it on Left and
  Right.
- `Toggle` draws an on/off switch.
- `Group` draws a foldable group's header, and its summary while folded.
- `OptionListState` and `OptionList` filter options by what is typed,
  matching name or hint regardless of case. Hints sit beside the names, or,
  narrow, the cursor's hint wraps below them; `Rows(w)` is the height to
  size a dialog by. With `Menu` there is no filter and `Pick` finds an
  option by its `Key`, shown before its name.
- `Frame` fills a region, borders it with its title, and returns the inside;
  sized with `Center` to what it holds, a dialog ends on its last line.
- `Rule` draws a line with a title and a hint, dropping the hint first.
- `Wires` lay lines on a grid by the arms that meet in each cell (`H`, `V`),
  and `DrawWires` draws every junction, tee and crossing joined.

### Scrollable list with scrollbar

```go
items := make([]tui.ListItem, 0, len(files))
for _, f := range files {
    items = append(items, tui.ListItem{
        Icon: '▸', IconFg: color.Amber,
        Text: f.Name, TextStyle: tui.Style{Fg: terminal.LightGray},
    })
}
r.List(items, state.Selection, state.Offset, tui.ListOpts{CursorBg: color.DarkSlate})
r.ScrollBar(r.W-1, state.Offset, state.Visible, state.Total, color.IronGray)
```

### Modal dialog

```go
dlg := tui.Center(root, 50, 12)
content := dlg.Modal(tui.ModalOpts{
    Title:    "Settings",
    Border:   tui.LineDouble,
    BorderFg: color.SteelBlue,
    TitleFg:  terminal.White,
    Bg:       color.DarkSlate,
})
content.TextBlock(0, 0, body, fg, color.DarkSlate, terminal.AttrNone)
```

`Modal` fills, borders, titles, and returns the content region. `Overlay`
adds fullscreen/floating/shadow variants; `ConfirmDialog` adds yes/no buttons
with focus state.

### Progress overlay

```go
prog := tui.NewProgressState(tui.DefaultProgressOpts("Indexing", "Scanning...",
    tui.ProgressDeterminate))

// per frame:
prog.Tick()
prog.SetProgress(done / total)
if prog.Visible {
    root.ProgressOverlay(prog.Opts)
}
```

Five progress types (spinner, determinate, indeterminate, pulse, dots), eight
spinner styles, eight bar styles, seven frame styles — combinable via opts.

### Multi-line editor

```go
ed := tui.NewEditorState(initialText)

// input:
if ev.Type == terminal.EventKey {
    ed.HandleKey(ev.Key, ev.Rune, ev.Modifiers) // full emacs-style bindings built in
}

// render:
r.Editor(ed, tui.EditorOpts{LineNumbers: true, Border: tui.LineSingle, Focused: true})
text := ed.Value()
```

`TextFieldState` + `TextField` provide the single-line equivalent
(placeholder, prefix, password mask, max length).

## State helpers

Pure logic, no rendering — usable independently:

- `ScrollState` — item-index scrolling with selection
  (`SelectNext/Prev`, `EnsureVisible`, `PageUp/Down`, `AtTop/AtBottom`)
- `ViewportScroll` — row-based content scrolling with viewport clipping
  (`ClipToViewport` maps content rows to visible rows, `EnsureRange` keeps a
  range in view); `Region.Window` draws content taller than the region off
  screen and shows the rows in view
- `TreeState` + `TreeExpansion` + `TreeBuilder` — cursor/scroll, expand/collapse
  keyed state, hierarchical → flat visible-node list
- `EditorState`, `TextFieldState` — text content, cursor, scroll, key handling
- `MasonryState` — multi-column layout calculation over a viewport
- Free functions: `AdjustScroll`, `ClampScroll`, `ClampCursor`, `ScrollPercent`,
  `PageDelta`

## Notes

- Width calculations count runes, not terminal columns; East Asian wide
  characters and combining marks are not width-aware.
- Zero-value `color.RGB` in style fields generally means "inherit"
  (widget default or row background) — check specific widget docs. A zero
  background with no Bg color bits is transparent: the cell keeps the
  background beneath it, bits and all.
- Mouse hit testing: `TabBar` returns `[]TabBounds`; other widgets require
  application-side geometry from the regions used.
