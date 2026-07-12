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
    root.Fill(terminal.Gunmetal)

    root.Box(tui.LineRounded, terminal.SteelBlue)
    content := root.Inset(1)
    content.List(buildItems(items), list.Selection, list.Offset, tui.ListOpts{
        CursorBg: terminal.DarkSlate,
    })
    content.ScrollBar(content.W-1, list.Offset, list.Visible, list.Total,
        terminal.IronGray)

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
`PadCenter`, `WrapText`.

`Style{Fg, Bg, Attr}` bundles cell appearance; most widget option structs
accept it.

## Widgets

Widgets are stateless render functions (mostly `Region` methods). Application
state lives in plain structs passed by pointer. Available renderers:

boxes and lines (`Box`, `BoxFilled`, `HLine`, `VLine` — single, double,
rounded, heavy line types), `List`, `Table`, `Tree`, `TabBar`, `KeyValue` /
`KeyValueWrap`, `Progress` / `ProgressV` / `Gauge` / `Spinner`,
`ProgressOverlay`, `Sparkline` / `SparklineV`, `Input` / `TextField`,
`Editor`, `Modal` / `Overlay` / `ConfirmDialog`, `ScrollBar` /
`ScrollIndicator`, masonry layout.

Representative patterns below; remaining widgets follow the same
opts-struct + state-struct shape — read the source for full options.

### Scrollable list with scrollbar

```go
items := make([]tui.ListItem, 0, len(files))
for _, f := range files {
    items = append(items, tui.ListItem{
        Icon: '▸', IconFg: terminal.Amber,
        Text: f.Name, TextStyle: tui.Style{Fg: terminal.LightGray},
    })
}
r.List(items, state.Selection, state.Offset, tui.ListOpts{CursorBg: terminal.DarkSlate})
r.ScrollBar(r.W-1, state.Offset, state.Visible, state.Total, terminal.IronGray)
```

### Modal dialog

```go
dlg := tui.Center(root, 50, 12)
content := dlg.Modal(tui.ModalOpts{
    Title:    "Settings",
    Border:   tui.LineDouble,
    BorderFg: terminal.SteelBlue,
    TitleFg:  terminal.White,
    Bg:       terminal.DarkSlate,
})
content.TextBlock(0, 0, body, fg, terminal.DarkSlate, terminal.AttrNone)
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
  (`ClipToViewport` maps content rows to visible rows)
- `TreeState` + `TreeExpansion` + `TreeBuilder` — cursor/scroll, expand/collapse
  keyed state, hierarchical → flat visible-node list
- `EditorState`, `TextFieldState` — text content, cursor, scroll, key handling
- `MasonryState` — multi-column layout calculation over a viewport
- Free functions: `AdjustScroll`, `ClampScroll`, `ClampCursor`, `ScrollPercent`,
  `PageDelta`

## Notes

- Width calculations count runes, not terminal columns; East Asian wide
  characters and combining marks are not width-aware.
- Zero-value `terminal.RGB` in style fields generally means "inherit"
  (widget default or row background) — check specific widget docs.
- Mouse hit testing: `TabBar` returns `[]TabBounds`; other widgets require
  application-side geometry from the regions used.
