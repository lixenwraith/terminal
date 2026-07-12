//go:build unix

package inline

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lixenwraith/terminal"
)

// Style describes text appearance; zero value is unstyled.
// Composable: inline.Fg(terminal.Amber).Attr(terminal.AttrBold)
type Style struct {
	fg, bg       terminal.RGB
	hasFg, hasBg bool
	attr         terminal.Attr
}

// Fg starts a style with foreground color
func Fg(c terminal.RGB) Style { return Style{fg: c, hasFg: true} }

// Bg sets background color
func (s Style) Bg(c terminal.RGB) Style { s.bg, s.hasBg = c, true; return s }

// Attr adds attribute bits
func (s Style) Attr(a terminal.Attr) Style { s.attr |= a; return s }

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
		bit  terminal.Attr
		code string
	}{
		{terminal.AttrBold, ";1"}, {terminal.AttrDim, ";2"},
		{terminal.AttrItalic, ";3"}, {terminal.AttrUnderline, ";4"},
		{terminal.AttrBlink, ";5"}, {terminal.AttrReverse, ";7"},
	} {
		if s.attr&m.bit != 0 {
			b.WriteString(m.code)
		}
	}
	if s.hasFg {
		if p.mode == terminal.ColorModeTrueColor {
			fmt.Fprintf(b, ";38;2;%d;%d;%d", s.fg.R, s.fg.G, s.fg.B)
		} else {
			fmt.Fprintf(b, ";38;5;%d", terminal.RGBTo256(s.fg))
		}
	}
	if s.hasBg {
		if p.mode == terminal.ColorModeTrueColor {
			fmt.Fprintf(b, ";48;2;%d;%d;%d", s.bg.R, s.bg.G, s.bg.B)
		} else {
			fmt.Fprintf(b, ";48;5;%d", terminal.RGBTo256(s.bg))
		}
	}
	b.WriteByte('m')
}

// --- Width handling (internal, rune-count semantics) ---
// Handles only 'm'-terminated escapes — this package's own SGR output.

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
