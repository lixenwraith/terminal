package terminal

import "github.com/lixenwraith/color"

// ColorMode indicates terminal color capability
type ColorMode uint8

const (
	ColorMode256       ColorMode = iota // xterm-256 palette
	ColorModeTrueColor                  // 24-bit RGB
	ColorMode16                         // ANSI 16: SGR 30-37/90-97, 40-47/100-107
	ColorModeNone                       // no color; attributes only
)

// WarmPalette256 delegates to the color package's lazily evaluated 256-color LUT builder.
// Exists to preserve terminal API backwards-compatibility.
func WarmPalette256() {
	color.WarmXterm256()
}

// RGBTo256 delegates to the color package's perceptual quantizer.
// Exists to preserve terminal API backwards-compatibility.
func RGBTo256(c color.RGB) uint8 {
	return color.RGBTo256(c)
}
