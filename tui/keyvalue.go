package tui

import (
	"github.com/lixenwraith/terminal"
)

// keyValueSplit divides a row between the key and value columns, excluding the
// separator. Each column takes the width it needs and yields only when that
// would push the other below a third of the row.
func keyValueSplit(width, keyLen, valLen int) (keyW, valW int) {
	avail := width - 1 // Separator column
	if avail < 2 {
		return 1, 1
	}
	third := max(avail/3, 1)

	keyW = max(keyLen, 1)
	if valFloor := min(valLen, third); avail-keyW < valFloor {
		keyW = avail - valFloor
	}
	keyW = min(max(keyW, min(keyLen, third)), avail-1)
	return keyW, avail - keyW
}

// KeyValue renders right-aligned key, separator and left-aligned value on row y,
// sizing the key column to this row alone
func (r Region) KeyValue(y int, key, value string, keyStyle, valStyle Style, sep rune) {
	r.KeyValueColumn(y, 0, key, value, keyStyle, valStyle, sep)
}

// KeyValueColumn renders a key-value row against an explicit key column, so a
// set of rows aligns on the separator; keyW <= 0 sizes the column to this row
func (r Region) KeyValueColumn(y, keyW int, key, value string, keyStyle, valStyle Style, sep rune) {
	if y < 0 || y >= r.H || r.W < 3 {
		return
	}

	var valW int
	if keyW <= 0 {
		keyW, valW = keyValueSplit(r.W, RuneLen(key), RuneLen(value))
	} else {
		keyW = min(max(keyW, 1), r.W-2)
		valW = r.W - keyW - 1
	}

	key = Truncate(key, keyW)
	value = Truncate(value, valW)

	// Key is right-aligned within its column, so the separators line up when
	// callers share a column width
	r.TextStyled(keyW-RuneLen(key), y, key, keyStyle)
	r.Cell(keyW, y, sep, keyStyle.Fg, keyStyle.Bg, terminal.AttrDim)
	r.TextStyled(keyW+1, y, value, valStyle)
}

// KeyValueWrap renders a key with its value wrapped into the value column,
// returning the number of lines used
//
//	key: value text that is
//	     long and wraps to
//	     next line
func (r Region) KeyValueWrap(y int, key, value string, keyStyle, valStyle Style, sep rune) int {
	if y < 0 || y >= r.H || r.W < 3 {
		return 0
	}

	keyW, valW := keyValueSplit(r.W, RuneLen(key), RuneLen(value))
	k := Truncate(key, keyW)

	r.TextStyled(keyW-RuneLen(k), y, k, keyStyle)
	r.Cell(keyW, y, sep, keyStyle.Fg, keyStyle.Bg, terminal.AttrDim)

	rendered := 0
	for i, line := range WrapText(value, valW) {
		if y+i >= r.H {
			break
		}
		r.TextStyled(keyW+1, y+i, line, valStyle)
		rendered++
	}
	return max(rendered, 1)
}

// MeasureKeyValueWrap returns the line count KeyValueWrap needs, for layout
// pre-calculation. Shares the column split with the renderer, so the two agree.
func (r Region) MeasureKeyValueWrap(key, value string) int {
	if r.W < 3 {
		return 1
	}
	_, valW := keyValueSplit(r.W, RuneLen(key), RuneLen(value))
	return max(len(WrapText(value, valW)), 1)
}

