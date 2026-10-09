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

// paletteTo16 maps an xterm-256 palette index to the nearest ANSI 16 index:
// the first 16 are themselves, the cube and the grey ramp go by their shade
func paletteTo16(i uint8) uint8 {
	switch {
	case i < 16:
		return i
	case i >= 232:
		v := 8 + 10*(i-232)
		return color.RGBTo16(color.RGB{R: v, G: v, B: v})
	}
	r, g, b := color.CubeRGB256(i)
	return color.RGBTo16(color.RGB{R: cubeLevel[r], G: cubeLevel[g], B: cubeLevel[b]})
}

// cubeLevel is the channel value of each xterm color cube step
var cubeLevel = [6]uint8{0, 95, 135, 175, 215, 255}

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
