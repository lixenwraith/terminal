package inline

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lixenwraith/color"
)

// Attribute represents visual text modifiers for inline styling
type Attribute uint8

const (
	AttributeNone Attribute = 0
	Bold          Attribute = 1 << 0
	Dim           Attribute = 1 << 1
	Italic        Attribute = 1 << 2
	Underline     Attribute = 1 << 3
	Blink         Attribute = 1 << 4
	Reverse       Attribute = 1 << 5
)

// Style describes text appearance; zero value is unstyled.
// Composable: inline.Fg(color.Amber).Bold().Underline()
type Style struct {
	fg, bg paint
	attr   Attribute
}

// paint is one color: none, an RGB value, or an ANSI 16 index
type paint struct {
	rgb  color.RGB
	ansi uint8
	kind uint8 // paintNone, paintRGB, paintANSI
}

const (
	paintNone uint8 = iota
	paintRGB
	paintANSI
)

// Fg starts a style with foreground color
func Fg(c color.RGB) Style { return Style{fg: paint{rgb: c, kind: paintRGB}} }

// FgANSI starts a style with an ANSI 16 foreground (color.ANSIRed..). The
// terminal's theme picks the shade, and every terminal, a text console
// included, renders it.
func FgANSI(i uint8) Style { return Style{fg: paint{ansi: i & 15, kind: paintANSI}} }

// Bg sets background color
func (s Style) Bg(c color.RGB) Style { s.bg = paint{rgb: c, kind: paintRGB}; return s }

// BgANSI sets an ANSI 16 background
func (s Style) BgANSI(i uint8) Style { s.bg = paint{ansi: i & 15, kind: paintANSI}; return s }

// Bold applies the bold attribute
func (s Style) Bold() Style { s.attr |= Bold; return s }

// Dim applies the dim/faint attribute
func (s Style) Dim() Style { s.attr |= Dim; return s }

// Italic applies the italic attribute
func (s Style) Italic() Style { s.attr |= Italic; return s }

// Underline applies the underline attribute
func (s Style) Underline() Style { s.attr |= Underline; return s }

// Blink applies the blink attribute
func (s Style) Blink() Style { s.attr |= Blink; return s }

// Reverse applies the reverse video attribute
func (s Style) Reverse() Style { s.attr |= Reverse; return s }

// Paint returns s styled for the detected terminal, unchanged when color
// is disabled. Composes with Log: p.Log("%s %s", p.Paint("ok", st), name)
func (p *Printer) Paint(s string, st Style) string {
	if !p.color {
		return s
	}
	var b strings.Builder
	p.writeSGR(&b, st)
	b.WriteString(s)
	b.WriteString("\x1b[0m")
	return b.String()
}

func (p *Printer) writeSGR(b *strings.Builder, s Style) {
	b.WriteString("\x1b[0")
	for _, m := range [...]struct {
		bit  Attribute
		code string
	}{
		{Bold, ";1"}, {Dim, ";2"},
		{Italic, ";3"}, {Underline, ";4"},
		{Blink, ";5"}, {Reverse, ";7"},
	} {
		if s.attr&m.bit != 0 {
			b.WriteString(m.code)
		}
	}
	p.writeColor(b, s.fg, 30)
	p.writeColor(b, s.bg, 40)
	b.WriteByte('m')
}

// writeColor emits one color for base 30 (foreground) or 40 (background).
// ANSI indices go out as 30-37/90-97 in every mode; RGB as the mode allows,
// degraded to the nearest ANSI index on a 16-color terminal.
func (p *Printer) writeColor(b *strings.Builder, c paint, base int) {
	if c.kind == paintRGB {
		switch p.mode {
		case colorModeTrueColor:
			fmt.Fprintf(b, ";%d;2;%d;%d;%d", base+8, c.rgb.R, c.rgb.G, c.rgb.B)
			return
		case colorMode256:
			fmt.Fprintf(b, ";%d;5;%d", base+8, color.RGBTo256(c.rgb))
			return
		}
		c = paint{ansi: color.RGBTo16(c.rgb), kind: paintANSI}
	}
	if c.kind == paintANSI {
		if c.ansi < 8 {
			fmt.Fprintf(b, ";%d", base+int(c.ansi))
		} else {
			fmt.Fprintf(b, ";%d", base+60+int(c.ansi)-8)
		}
	}
}

// --- Width handling (internal, rune-count semantics) ---

// visibleLen counts runes excluding SGR sequences
func visibleLen(s string) int {
	n := 0
	for {
		i := strings.IndexByte(s, 0x1b)
		if i < 0 {
			return n + utf8.RuneCountInString(s)
		}
		n += utf8.RuneCountInString(s[:i])
		m := strings.IndexByte(s[i:], 'm')
		if m < 0 {
			return n // Unterminated escape, remainder not visible
		}
		s = s[i+m+1:]
	}
}

// runePrefix returns up to k leading runes of s and the count taken
func runePrefix(s string, k int) (string, int) {
	if k <= 0 {
		return "", 0
	}
	n := 0
	for i := range s {
		if n == k {
			return s[:i], n
		}
		n++
	}
	return s, n
}

// truncVisible truncates to max visible runes, preserving embedded SGR
// sequences and appending a reset when cut
func truncVisible(s string, max int) string {
	if visibleLen(s) <= max {
		return s
	}
	var b strings.Builder
	n := 0
	for len(s) > 0 {
		i := strings.IndexByte(s, 0x1b)
		if i != 0 {
			seg := s
			if i > 0 {
				seg = s[:i]
			}
			pre, taken := runePrefix(seg, max-n)
			b.WriteString(pre)
			n += taken
			if n >= max {
				break
			}
			s = s[len(seg):]
			continue
		}
		m := strings.IndexByte(s, 'm')
		if m < 0 {
			break // Unterminated escape, drop remainder
		}
		b.WriteString(s[:m+1])
		s = s[m+1:]
	}
	b.WriteString("\x1b[0m")
	return b.String()
}
