//go:build unix

package inline

import "strings"

// BarBlock is the default bar character set [filled, partial, empty]
var BarBlock = [3]rune{'█', '▌', '░'}

// Bar renders an unstyled progress bar of width cells, pct in [0,1]
func Bar(width int, pct float64, chars [3]rune) string {
	if width < 1 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(float64(width) * pct)
	rem := float64(width)*pct - float64(filled)

	var b strings.Builder
	b.Grow(width * 3) // Worst-case UTF-8
	for i := range width {
		switch {
		case i < filled:
			b.WriteRune(chars[0])
		case i == filled && rem >= 0.5 && filled < width:
			b.WriteRune(chars[1])
		default:
			b.WriteRune(chars[2])
		}
	}
	return b.String()
}

// Braille frames; intentionally duplicated from tui (no tui dependency)
var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner returns the frame for a monotonic counter
func Spinner(frame int) string {
	i := frame % len(spinnerFrames)
	if i < 0 {
		i = -i
	}
	return spinnerFrames[i]
}
