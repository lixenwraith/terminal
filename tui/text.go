package tui

import (
	"strings"
	"unicode/utf8"
)

// RuneLen returns s's width in cells, a rune each: Flush draws every rune in
// one column, a wide or joining one as U+FFFD
func RuneLen(s string) int {
	return utf8.RuneCountInString(s)
}

// Truncate truncates string with … suffix if exceeds maxLen
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	// Boundary-safe truncation for UTF-8
	count := 0
	for i := range s {
		if count == maxLen-1 {
			return s[:i] + "…"
		}
		count++
	}
	return s
}

// TruncateLeft truncates with … prefix, keeps end of string
func TruncateLeft(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-maxLen+1:])
}

// TruncateMiddle keeps start and end, … in middle
func TruncateMiddle(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return Truncate(s, maxLen)
	}

	startLen := (maxLen - 1) / 2
	endLen := maxLen - 1 - startLen

	return string(runes[:startLen]) + "…" + string(runes[len(runes)-endLen:])
}

// PadRight pads string with spaces to width
func PadRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	result := make([]rune, width)
	copy(result, runes)
	for i := len(runes); i < width; i++ {
		result[i] = ' '
	}
	return string(result)
}

// PadLeft left-pads string with spaces to width
func PadLeft(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	result := make([]rune, width)
	padding := width - len(runes)
	for i := 0; i < padding; i++ {
		result[i] = ' '
	}
	copy(result[padding:], runes)
	return string(result)
}

// PadCenter centers string within width
func PadCenter(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	result := make([]rune, width)
	leftPad := (width - len(runes)) / 2
	for i := range result {
		result[i] = ' '
	}
	copy(result[leftPad:], runes)
	return string(result)
}

// WrapText wraps text to fit width: at each line break, at spaces, or, in a
// word longer than a line, after its last '.', ',', ':', '/', '[' or ']'
// that fits (a path or a key such as a[0].b); a word without one is cut.
// Returns slice of lines, each no longer than width.
func WrapText(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		lines = append(lines, wrapLine(line, width)...)
	}
	return lines
}

// wrapLine wraps one line, s holding no line break
func wrapLine(s string, width int) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return []string{""}
	}

	var lines []string
	lineStart := 0
	lastSpace, lastMark := -1, -1

	for i := 0; i <= len(runes); i++ {
		if i-lineStart >= width || i == len(runes) {
			if i == len(runes) {
				if lineStart < len(runes) {
					lines = append(lines, string(runes[lineStart:]))
				}
				break
			}

			wrapAt := i
			switch {
			case lastSpace > lineStart:
				wrapAt = lastSpace
			case lastMark > lineStart:
				wrapAt = lastMark
			}

			lines = append(lines, string(runes[lineStart:wrapAt]))

			if wrapAt < len(runes) && runes[wrapAt] == ' ' {
				lineStart = wrapAt + 1
			} else {
				lineStart = wrapAt
			}
			lastSpace, lastMark = -1, -1
		}

		// Track spaces for word wrapping, and the marks a long word breaks after
		if i < len(runes) && runes[i] == ' ' {
			lastSpace = i
		}
		if i < len(runes) && strings.ContainsRune(".,:/[]", runes[i]) {
			lastMark = i + 1
		}
	}

	if len(lines) == 0 {
		lines = []string{""}
	}

	return lines
}

// RepeatRune returns a string of n repeated runes, for string repeat use strings.Repeat
func RepeatRune(r rune, n int) string {
	if n <= 0 {
		return ""
	}
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = r
	}
	return string(runes)
}
