package tui

import (
	"github.com/lixenwraith/terminal"
)

// KeyValue renders right-aligned key, separator, left-aligned value on row
// Key width auto-sizes based on content, capped at 40% of region width
// Value gets remainder, minimum 30% of region width
func (r Region) KeyValue(y int, key, value string, keyStyle, valStyle Style, sep rune) {
	if y < 0 || y >= r.H || r.W < 3 {
		return
	}

	keyLen := RuneLen(key)

	// Dynamic allocation: key gets what it needs up to 40%
	maxKeyW := (r.W * 2) / 5  // 40%
	minValW := (r.W * 3) / 10 // 30%

	keyW := keyLen
	if keyW > maxKeyW {
		keyW = maxKeyW
	}
	if keyW < 1 {
		keyW = 1
	}

	valW := r.W - keyW - 1 // -1 for separator
	if valW < minValW && r.W > minValW+2 {
		// Reclaim from key to meet minimum value width
		valW = minValW
		keyW = r.W - valW - 1
		if keyW < 1 {
			keyW = 1
			valW = r.W - 2
		}
	}
	if valW < 1 {
		valW = 1
	}

	// Truncate key if needed
	keyRunes := []rune(key)
	if len(keyRunes) > keyW {
		if keyW > 1 {
			keyRunes = keyRunes[:keyW-1]
			keyRunes = append(keyRunes, '…')
		} else {
			keyRunes = keyRunes[:1]
		}
	}

	// Truncate value if needed
	valRunes := []rune(value)
	if len(valRunes) > valW {
		if valW > 1 {
			valRunes = valRunes[:valW-1]
			valRunes = append(valRunes, '…')
		} else {
			valRunes = valRunes[:1]
		}
	}

	// Right-align key within allocated width
	keyX := keyW - len(keyRunes)
	for i, ch := range keyRunes {
		r.Cell(keyX+i, y, ch, keyStyle.Fg, keyStyle.Bg, keyStyle.Attr)
	}

	// Separator
	r.Cell(keyW, y, sep, keyStyle.Fg, keyStyle.Bg, terminal.AttrDim)

	// Left-align value
	for i, ch := range valRunes {
		r.Cell(keyW+1+i, y, ch, valStyle.Fg, valStyle.Bg, valStyle.Attr)
	}
}

// KeyValueWrap renders key-value with value wrapping to subsequent lines
// Returns number of lines used
// Layout:
//
//	key: value text that is
//	     long and wraps to
//	     next line
func (r Region) KeyValueWrap(y int, key, value string, keyStyle, valStyle Style, sep rune) int {
	if y < 0 || y >= r.H || r.W < 3 {
		return 0
	}

	keyLen := RuneLen(key)

	// Dynamic allocation same as KeyValue
	maxKeyW := (r.W * 2) / 5  // 40%
	minValW := (r.W * 3) / 10 // 30%

	keyW := keyLen
	if keyW > maxKeyW {
		keyW = maxKeyW
	}
	if keyW < 1 {
		keyW = 1
	}

	valW := r.W - keyW - 1 // -1 for separator
	if valW < minValW && r.W > minValW+2 {
		valW = minValW
		keyW = r.W - valW - 1
		if keyW < 1 {
			keyW = 1
			valW = r.W - 2
		}
	}
	if valW < 1 {
		valW = 1
	}

	// Truncate key if needed
	keyRunes := []rune(key)
	if len(keyRunes) > keyW {
		if keyW > 1 {
			keyRunes = keyRunes[:keyW-1]
			keyRunes = append(keyRunes, '…')
		} else {
			keyRunes = keyRunes[:1]
		}
	}

	// Right-align key within allocated width
	keyX := keyW - len(keyRunes)
	for i, ch := range keyRunes {
		r.Cell(keyX+i, y, ch, keyStyle.Fg, keyStyle.Bg, keyStyle.Attr)
	}

	// Separator
	r.Cell(keyW, y, sep, keyStyle.Fg, keyStyle.Bg, terminal.AttrDim)

	// Wrap value text
	valueX := keyW + 1
	lines := WrapText(value, valW)
	if len(lines) == 0 {
		return 1
	}

	rendered := 0
	for i, line := range lines {
		lineY := y + i
		if lineY >= r.H {
			break
		}
		r.Text(valueX, lineY, line, valStyle.Fg, valStyle.Bg, valStyle.Attr)
		rendered++
	}

	if rendered < 1 {
		rendered = 1
	}
	return rendered
}

// MeasureKeyValueWrap calculates lines needed for KeyValueWrap without rendering
// Useful for layout pre-calculation
func (r Region) MeasureKeyValueWrap(key, value string) int {
	if r.W < 3 {
		return 1
	}

	keyLen := RuneLen(key)
	maxKeyW := (r.W * 2) / 5
	minValW := (r.W * 3) / 10

	keyW := keyLen
	if keyW > maxKeyW {
		keyW = maxKeyW
	}
	if keyW < 1 {
		keyW = 1
	}

	valW := r.W - keyW - 1
	if valW < minValW && r.W > minValW+2 {
		valW = minValW
		keyW = r.W - valW - 1
		if keyW < 1 {
			keyW = 1
			valW = r.W - 2
		}
	}
	if valW < 1 {
		valW = 1
	}

	lines := WrapText(value, valW)
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}