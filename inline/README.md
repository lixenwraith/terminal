# inline

Styled text and in-place progress in the normal terminal scrollback. No raw
mode, no alternate screen, no input handling, no cursor hiding — the shell
keeps owning the terminal. Intended for CLI tools (package managers, service
tooling, build scripts) that want color and live status without a full-screen
TUI.

Unix only (`//go:build unix`). Depends on the parent `terminal` package for
color types, capability detection, and RGB → 256 mapping.

## Model

Output is split into two zones:

    installed openssl          ← permanent lines (Log) — scroll normally
    installed zlib
    ⠹ installing curl          ← live block (Update) — rewritten in place
    [██████░░░░░░░░] 2/5

`Log` prints permanent lines above the live block; `Update` replaces the live
block by cursor-up + clear + rewrite; `Done` erases the block and optionally
prints final lines. Interleaving is handled internally — `Log` during an
active live block erases, prints, and redraws in one flush.

## API

### Printer

| Method | Description |
|---|---|
| `New(w io.Writer) *Printer` | Creates a printer. Terminal detection via size probe; color defaults on for terminals with `NO_COLOR` unset. Safe for concurrent use. |
| `Log(format string, a ...any)` | Prints one permanent line above the live block (`Printf` semantics, newline appended). |
| `Update(lines ...string)` | Replaces the live block, rewriting in place. No-op on non-terminal output. |
| `Done(final ...string)` | Erases the live block and prints final permanent lines. Call before exit. |
| `Paint(s string, st Style) string` | Returns `s` wrapped in SGR codes for the detected color mode, or unchanged when color is off. |
| `SetColor(on bool)` | Overrides color detection (e.g. force styling into a pipe for `less -R`). Affects `Paint` only; `Update` remains terminal-gated. |
| `Size() (w, h int)` | Current terminal dimensions, 80×24 when unknown. |

### Style

Value type, zero value is unstyled, builder-composable:

| Function | Description |
|---|---|
| `Fg(c terminal.RGB) Style` | Starts a style with foreground color. |
| `(s Style) Bg(c terminal.RGB) Style` | Adds background color. |
| `(s Style) Attr(a terminal.Attr) Style` | Adds attribute bits (`AttrBold`, `AttrDim`, ...). |

```go
warn := inline.Fg(terminal.Amber).Attr(terminal.AttrBold)
p.Log("%s low disk space", p.Paint("warning:", warn))
```

True color terminals get `38;2;R;G;B`; 256-color terminals get `38;5;N` via
Redmean mapping — same degradation path as the parent package.

### Progress helpers

Pure string builders, no Printer required:

| Function | Description |
|---|---|
| `Bar(width int, pct float64, chars [3]rune) string` | Progress bar of `width` cells, `pct` clamped to [0,1], half-cell resolution via the partial rune. |
| `BarBlock` | Default character set `[3]rune{'█', '▌', '░'}`. |
| `Spinner(frame int) string` | Braille spinner frame for a monotonic counter. |

Compose with `Paint` for colored bars:

```go
line := "[" + p.Paint(inline.Bar(30, pct, inline.BarBlock), barStyle) + "]"
```

## Non-terminal output

When output is a pipe or file (CI, redirection): `Update` is a no-op, `Paint`
returns input unchanged, `Log` and `Done` print plain sequential text. A tool
using inline degrades to ordinary log output with no code changes.

## Example

Simulated package installation — spinner, overall progress bar, permanent
completion lines:

```go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/inline"
)

func main() {
	p := inline.New(os.Stdout)

	name := inline.Fg(terminal.LightSkyBlue).Attr(terminal.AttrBold)
	okSt := inline.Fg(terminal.LimeGreen).Attr(terminal.AttrBold)
	dim := inline.Fg(terminal.IronGray)

	pkgs := []string{"openssl", "zlib", "curl", "git", "go"}
	frame := 0

	for i, pkg := range pkgs {
		const steps = 25
		for s := range steps {
			pct := (float64(i) + float64(s)/steps) / float64(len(pkgs))
			p.Update(
				inline.Spinner(frame)+" installing "+p.Paint(pkg, name),
				"["+inline.Bar(32, pct, inline.BarBlock)+"] "+
					p.Paint(fmt.Sprintf("%d/%d", i+1, len(pkgs)), dim),
			)
			frame++
			time.Sleep(40 * time.Millisecond)
		}
		p.Log("%s %s", p.Paint("✓", okSt), pkg)
	}

	p.Done(p.Paint("✓ 5 packages installed", okSt))
}
```

Run in a terminal: the two-line status block animates in place while
completion lines accumulate above it. Piped (`go run . | cat`): only the
completion lines and the final summary appear, unstyled.

## Notes

- Width is measured in runes (`unicode/utf8`); East Asian wide characters and
  combining marks are not width-aware — same limitation as `tui`.
- Live block lines must occupy one visual row each: no `\n`, tabs, or control
  characters. Lines are truncated to terminal width automatically; embedded
  SGR from `Paint` is preserved through truncation.
- The live block is clamped to terminal height − 1 rows (newest lines kept).
- Pass external strings (package names, paths) as `Log` arguments, never as
  the format string.
- Ctrl-C mid-update leaves the live block on screen but the terminal in a
  normal state — no raw mode or screen buffer to restore.
