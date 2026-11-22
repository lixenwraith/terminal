#!/usr/bin/env python3
"""Bake Hack into what package atlas embeds. `bake` renders the TrueType at each
strike's cell size into the packed font file package glyph parses, and subsets it
to a woff2 of the same code points; `dump` prints a font file as rows of `.` and
`#`, the form a reviewer diffs. Needs Pillow, fontTools and brotli."""

from __future__ import annotations

import argparse
import hashlib
import io
import os
import struct
import sys
import unicodedata
import urllib.request
from pathlib import Path
from typing import NamedTuple

from fontTools import subset
from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFont

URL = "https://raw.githubusercontent.com/source-foundry/Hack/v3.003/build/ttf/Hack-Regular.ttf"
SHA256 = "15f55cc0c85a2988d2b4b3a8cdb5d77fdfbaf319e1bb5309d725db9818fb7125"
REPO = Path(__file__).resolve().parents[2]
MAGIC = b"TGF1"

# The general categories left out, with the wide and fullwidth runes: what a cell
# draws as a space or U+FFFD. Go's terminal.OneColumn is the
# authority: atlas's test holds the baked file to it
NOT_DRAWN = frozenset(("Mn", "Me", "Mc", "Cc", "Cf", "Zl", "Zp", "Cs", "Cn"))

# Box drawing and block elements join from cell to cell. The font's hinting moves
# a line by a row from one glyph to the next, so these are drawn without it, and
# box drawing is moved to where a light line falls on whole pixels
LINES = range(0x2500, 0x25A0)
BOX = range(0x2500, 0x2580)


class Draw(NamedTuple):
    size: float  # pixels an em, a whole count of 64ths: FreeType keeps no more
    threshold: int  # the coverage, of 255, from which a pixel is set


class Strike(NamedTuple):
    width: int
    height: int
    text: Draw
    lines: Draw
    shift: tuple[int, int]  # how far BOX moves right and up, in the font's units


# Smallest first, as the file orders them. Text takes the largest size at which no
# glyph that fits its advance loses a pixel off the cell, and the threshold that
# keeps the most strokes and counters. LINES take the size, threshold and shift
# at which an arm meets its edge on the same rows in every glyph, a corner grows
# no pixel, a double line keeps its gap and halves and quadrants tile.
STRIKES = (
    Strike(6, 12, Draw(10.4375, 80), Draw(10.75, 128), (16, -8)),
    Strike(8, 16, Draw(13.75, 80), Draw(13.75, 138), (28, -34)),
    Strike(12, 24, Draw(20, 96), Draw(20, 96), (26, 12)),
)


def verified(data: bytes, where: str) -> bytes:
    """data when it is Hack 3.003's TTF; anything else stops the tool"""
    got = hashlib.sha256(data).hexdigest()
    if got != SHA256:
        sys.exit(f"{where}: SHA-256 {got}, not {SHA256}")
    return data


def source(font: str | None, cache: Path) -> bytes:
    """The TTF: the file named, else the one the cache holds, downloaded once"""
    if font:
        return verified(Path(font).read_bytes(), font)
    path = cache / "Hack-Regular.ttf"
    if REPO in path.resolve().parents:
        sys.exit(f"{cache}: the TTF is not kept inside the repository")
    if not path.exists():
        with urllib.request.urlopen(URL) as response:
            data = verified(response.read(), URL)
        cache.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    return verified(path.read_bytes(), str(path))


def code_points(ttf: bytes) -> list[int]:
    """Every code point of the font's cmap a cell draws as itself, ascending"""
    return sorted(
        c
        for c in TTFont(io.BytesIO(ttf)).getBestCmap()
        if unicodedata.category(chr(c)) not in NOT_DRAWN and unicodedata.east_asian_width(chr(c)) not in "WF"
    )


def unhinted(ttf: bytes, shift: tuple[int, int]) -> bytes:
    """The TTF with no instructions on the glyphs of LINES, those of BOX moved by shift"""
    font = TTFont(io.BytesIO(ttf), recalcTimestamp=False)
    cmap, glyf, hmtx = font.getBestCmap(), font["glyf"], font["hmtx"]
    for c in LINES:
        if c not in cmap:
            continue
        glyph = glyf[cmap[c]]
        glyph.removeHinting()
        if c in BOX:
            glyph.coordinates.translate(shift)
            # the bearing moves with the outline, or FreeType moves the outline back
            hmtx[cmap[c]] = (hmtx[cmap[c]][0], hmtx[cmap[c]][1] + shift[0])
    out = io.BytesIO()
    font.save(out)
    return out.getvalue()


def face(ttf: bytes, size: float) -> ImageFont.FreeTypeFont:
    # the basic layout: one glyph a code point, whether or not Pillow has raqm
    return ImageFont.truetype(io.BytesIO(ttf), size, layout_engine=ImageFont.Layout.BASIC)


def render(font: ImageFont.FreeTypeFont, ch: str, width: int, height: int, threshold: int) -> bytes:
    """A glyph as the font file holds it: a bit a pixel, a row padded to a byte"""
    img = Image.new("L", (width, height), 0)
    ImageDraw.Draw(img).text((0, 0), ch, font=font, fill=255)
    return img.point(lambda v: 255 * (v >= threshold), "1").tobytes()


# The diamond suit: a text console's font holds it and Hack does not, so the tool
# draws it, for a program whose glyph must be one on every screen
DIAMOND = 0x2666


def diamond(width: int, height: int) -> bytes:
    """The suit as a rhombus about the cell's middle, in render's form"""
    img = Image.new("1", (width, height), 0)
    cx, cy, hw, hh = (width - 1) / 2, (height - 1) / 2, 0.42 * width, 0.3 * height
    for y in range(height):
        for x in range(width):
            img.putpixel((x, y), int(abs(x - cx) / hw + abs(y - cy) / hh <= 1))
    return img.tobytes()


def pack(ttf: bytes, runes: list[int]) -> bytes:
    """The font file of glyph's README: the face's name, the runes, then each strike"""
    name = " ".join(filter(None, face(ttf, 20).getname())).encode()
    out = bytearray(MAGIC + struct.pack("<HHI", len(STRIKES), len(name), len(runes)))
    out += name + bytes(-len(name) % 4)
    out += struct.pack(f"<{len(runes)}I", *runes)
    for strike in STRIKES:
        text, lines = face(ttf, strike.text.size), face(unhinted(ttf, strike.shift), strike.lines.size)
        out += struct.pack("<HH", strike.width, strike.height)
        for c in runes:
            if c == DIAMOND:
                out += diamond(strike.width, strike.height)
                continue
            font, draw = (lines, strike.lines) if c in LINES else (text, strike.text)
            out += render(font, chr(c), strike.width, strike.height, draw.threshold)
    return bytes(out)


def web(ttf: bytes, runes: list[int]) -> bytes:
    """The TTF as a woff2 of those code points. A page draws a code point a cell and
    scaled, so it loses its layout features, its hinting and ttfautohint's record
    of it; it keeps every name, the copyright and the license among them"""
    options = subset.Options(flavor="woff2", hinting=False, name_IDs=["*"])
    options.drop_tables += ["GSUB", "TTFA"]
    font = subset.load_font(io.BytesIO(ttf), options)
    subsetter = subset.Subsetter(options)
    subsetter.populate(unicodes=runes)
    subsetter.subset(font)
    out = io.BytesIO()
    subset.save_font(font, out, options)
    return out.getvalue()


def bake(args: argparse.Namespace) -> None:
    ttf = source(args.font, Path(args.cache))
    runes = code_points(ttf)
    out = Path(args.out)
    drawn = sorted({*runes, DIAMOND})
    for name, data in (("hack.bin", pack(ttf, drawn)), ("hack.woff2", web(ttf, runes))):
        (out / name).write_bytes(data)
        print(f"{out / name}: {len(data)} bytes")
    print(f"{len(drawn)} glyphs at " + " ".join(f"{s.width}x{s.height}" for s in STRIKES))


def dump(args: argparse.Namespace) -> None:
    data = Path(args.file).read_bytes()
    if data[:4] != MAGIC:
        sys.exit(f"{args.file}: no {MAGIC.decode()} header")
    count, named, glyphs = struct.unpack_from("<HHI", data, 4)
    at = 12 + (named + 3 & ~3)
    runes = struct.unpack_from(f"<{glyphs}I", data, at)
    at += 4 * glyphs
    strikes = []
    for _ in range(count):
        width, height = struct.unpack_from("<HH", data, at)
        strikes.append((width, height, at + 4))
        at += 4 + glyphs * height * ((width + 7) // 8)
    sizes = " ".join(f"{w}x{h}" for w, h, _ in strikes)
    print(f"{data[12:12 + named].decode()}: {glyphs} glyphs at {sizes}")
    for i, c in enumerate(runes):
        print(f"\nU+{c:04X} {chr(c)} {unicodedata.name(chr(c), '')}".rstrip())
        cells = [rows(data, start, width, height, i) for width, height, start in strikes]
        for y in range(max(map(len, cells))):
            print("  ".join(cell[y] if y < len(cell) else " " * len(cell[0]) for cell in cells).rstrip())


def rows(data: bytes, start: int, width: int, height: int, i: int) -> list[str]:
    """Glyph i of the strike whose glyphs start at start, a string a row"""
    stride = (width + 7) // 8
    out = []
    for at in range(start + i * height * stride, start + (i + 1) * height * stride, stride):
        bits = int.from_bytes(data[at : at + stride], "big")
        out.append(f"{bits:0{8 * stride}b}"[:width].translate({48: ".", 49: "#"}))
    return out


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    commands = ap.add_subparsers(required=True)
    cache = Path(os.environ.get("XDG_CACHE_HOME") or Path.home() / ".cache") / "font-atlas"
    b = commands.add_parser("bake", help="write hack.bin and hack.woff2")
    b.add_argument("--font", help=f"Hack-Regular.ttf of 3.003; without it, fetched to the cache from {URL}")
    b.add_argument("--cache", default=cache, help="where a fetched TTF is kept (default %(default)s)")
    b.add_argument("--out", default=REPO / "atlas", help="the directory written to (default %(default)s)")
    b.set_defaults(run=bake)
    d = commands.add_parser("dump", help="print a font file, a glyph at each strike side by side")
    d.add_argument("file")
    d.set_defaults(run=dump)
    args = ap.parse_args()
    args.run(args)


if __name__ == "__main__":
    main()
