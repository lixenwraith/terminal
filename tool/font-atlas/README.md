# font-atlas

Makes the two files package `atlas` embeds, from Hack 3.003's `Hack-Regular.ttf`:

- `atlas/hack.bin`: the face as bitmaps at 6x12, 8x16 and 12x24, in the font file
  `glyph/README.md` lays out.
- `atlas/hack.woff2`: the same code points as a web font, without layout features
  or hinting, its name table whole.

```bash
python3 tool/font-atlas/font-atlas.py bake                 # fetches the TTF to ~/.cache/font-atlas
python3 tool/font-atlas/font-atlas.py bake --font Hack-Regular.ttf
python3 tool/font-atlas/font-atlas.py dump atlas/hack.bin  # every glyph, its strikes side by side
```

Needs Python 3 with Pillow, fontTools and brotli. The TTF is held to its SHA-256
whether named or fetched, and a cache inside the repository is refused: the TTF is
never committed.

The code points are those of the font's cmap a cell draws as itself, 1,537 of
them: `terminal.OneColumn` decides and `atlas`'s test holds the file to it. The
tool reads the same from Python's Unicode tables; a code point those tables let
through and the test turns up is left out in the tool by name.

A strike is Pillow's text at a pixel size, a pixel set from a coverage threshold;
`STRIKES` holds both for each cell size. Box drawing and block elements have a
pair of their own and are drawn without the font's hinting, which would put one
glyph's line a row off its neighbour's; box drawing is moved besides, by a
fraction of a pixel, to where its lines fall on whole pixels. After a change to
`STRIKES`, review the diff of `dump` before and after: the test checks six lines
and the full block, and the eye the rest.

One glyph is the tool's own: the diamond suit, U+2666, which a text console's
font holds and Hack lacks, drawn as a rhombus so a program's glyph can be one on
every screen. It is in `hack.bin` alone: a page takes it from a font of its own.

The bits are FreeType's, so another Pillow may round a pixel the other way; the
files in `atlas` came from Pillow 12.3.0 (FreeType 2.14.3), fontTools 4.68.0 and
brotli 1.2.1.
