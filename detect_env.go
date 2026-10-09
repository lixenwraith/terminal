//go:build !wasm

package terminal

import "github.com/lixenwraith/terminal/internal/env"

// DetectColorMode determines terminal color capability from environment:
// 16 colors on text consoles, true color where announced, else 256. It
// reports what the terminal can show; New also honours NO_COLOR.
func DetectColorMode() ColorMode {
	switch env.Colors() {
	case 16:
		return ColorMode16
	case 1 << 24:
		return ColorModeTrueColor
	}
	return ColorMode256
}
