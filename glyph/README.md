# glyph

A bitmap font at several cell sizes, its strikes; the software raster that draws a
cell grid with one; and what a drawer in another runtime (a Java view, a shader)
paints from: a strike as one texture, and frames encoded for it. Every number below
is little-endian.

## The font file

`Parse` reads it where it lies and refuses any other.

```
offset  size             field
0       4                magic "TGF1"
4       2                n, the count of strikes, at least 1
6       2                l, the length of the name in bytes
8       4                g, the count of glyphs
12      l                the name, UTF-8, then zero bytes to the next multiple of 4
..      4*g              the runes, code points strictly ascending
then for each strike, strictly ascending by height:
        2                width in pixels, 1 to 32
        2                height in pixels, at least 1
        g*height*stride  the glyphs in rune order, a glyph's rows top first:
                         stride = ceil(width/8) bytes a row, bit 7 of the first
                         byte the leftmost column, the bits past the width zero
```

Every strike holds every rune; a glyph a strike does not draw is blank there.

## The texture

`NewTexture(strike).Bytes()`, a strike as a cell drawer uploads it:

```
offset     size   field
0          4      w, a cell's width in pixels
4          4      h, its height
8          4      s, the count of slots
12         s*w*h  the mask, a slot below the last: a slot's rows top first, a byte
                  a pixel, 255 where the glyph is set and 0 elsewhere
12+s*w*h   4*s    each slot's rune
```

A glyph with a set bit takes the next slot, in rune order; a blank one has none
and is painted as background. Each strike so has slots of its own.

## Frames

A `terminal.Frames` encodes for the drawer with one of two encoders. Colors are
`terminal.CellColors`: reverse swapped, dim halved, a palette index resolved.

`Texture.Frame`, the cells changed since the frame the drawer holds:

```
offset  size  field
0       4     cols
4       4     rows
8       4     c, the count of records
12      16*c  a record a cell, four words: its index, row-major; its glyph's
              slot, 0xffffffff for background only; its foreground and its
              background as 0xffRRGGBB
```

`Raster.Frame`, the picture itself, for a drawer that cannot paint cells:

```
offset  size             field
0       4                cols
4       4                rows
8       cols*w*rows*h*4  RGBA, rows top first, w by h the strike's cell
```

A cell drawer is checked with `NewProbe(tex, cols)`: `Probe.Frame`, every glyph
with the color rules taken in turn, painted at the strike's size is
`Probe.Pixels`, the raster's picture of it.

## Picking a strike

`Font.Cell(density, dp)` is the pixels of a cell `dp` tall on a screen of `density`
pixels a dp, whole ones at the face's aspect, and the strike that draws it: the
largest no taller than the cell, so a glyph is stretched up and never down in a
font whose strikes share one aspect. With no zoom kept, a window draws at the
face's own size, its largest strike's, and an Android view at
`android.DefaultCell`. `Zoom.Scale` steps a zoom within its range and stops at
the smallest strike; the zero `Zoom` keeps the cell. When a zoom crosses a
strike, the host makes that strike's `Texture` or `Raster`, and one whose drawer
runs elsewhere hands its encoder to `Frames.SetEncoder`: the next frame is whole,
in the new slots.
