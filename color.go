package terminal

// ColorMode indicates terminal color capability
type ColorMode uint8

const (
	ColorMode256       ColorMode = iota // xterm-256 palette
	ColorModeTrueColor                  // 24-bit RGB
)

// RGB represents a 24-bit color
type RGB struct {
	R uint8 `toml:"r"`
	G uint8 `toml:"g"`
	B uint8 `toml:"b"`
}

// Lerp linearly interpolates from c to other by t, clamped to [0,1]
func (c RGB) Lerp(other RGB, t float64) RGB {
	if t <= 0 {
		return c
	}
	if t >= 1 {
		return other
	}
	return RGB{
		R: clampU8(float64(c.R) + (float64(other.R)-float64(c.R))*t),
		G: clampU8(float64(c.G) + (float64(other.G)-float64(c.G))*t),
		B: clampU8(float64(c.B) + (float64(other.B)-float64(c.B))*t),
	}
}

// RGBBlack is the zero value black color
var RGBBlack = RGB{0, 0, 0}

// 6-bit quantized LUT for Redmean-based 256-color mapping
// 64×64×64 = 262,144 bytes, fits in L2 cache
var lut256 [64 * 64 * 64]uint8

func init() {
	// Pre-compute Redmean-based palette mapping for all 6-bit quantized RGB values
	for r := 0; r < 64; r++ {
		for g := 0; g < 64; g++ {
			for b := 0; b < 64; b++ {
				// Expand 6-bit to 8-bit (shift left 2, add 2 for midpoint)
				r8 := (r << 2) | 2
				g8 := (g << 2) | 2
				b8 := (b << 2) | 2
				lut256[r<<12|g<<6|b] = computeRedmean256(r8, g8, b8)
			}
		}
	}
}

// computeRedmean256 finds the nearest 256-palette index using Redmean distance
// Called only at init() to populate LUT
func computeRedmean256(r, g, b int) uint8 {
	// Grayscale fast path
	if r == g && g == b {
		if r < 8 {
			return 16
		}
		if r > 238 {
			return 231
		}
		return uint8(232 + (r-8)/10)
	}

	bestIdx := uint8(16)
	minDist := 1 << 30

	// Search 6×6×6 cube (indices 16-231)
	for i := 0; i < 216; i++ {
		cr := cubeValues[i/36]
		cg := cubeValues[(i/6)%6]
		cb := cubeValues[i%6]

		d := redmeanDistance(r, g, b, cr, cg, cb)
		if d < minDist {
			minDist = d
			bestIdx = uint8(16 + i)
		}
	}

	// Search grayscale ramp (indices 232-255)
	for i := 0; i < 24; i++ {
		gray := 8 + i*10
		d := redmeanDistance(r, g, b, gray, gray, gray)
		if d < minDist {
			minDist = d
			bestIdx = uint8(232 + i)
		}
	}

	return bestIdx
}

// redmeanDistance calculates perceptually-weighted color distance
// Formula: https://en.wikipedia.org/wiki/Color_difference#sRGB
func redmeanDistance(r1, g1, b1, r2, g2, b2 int) int {
	rmean := (r1 + r2) / 2
	dr := r1 - r2
	dg := g1 - g2
	db := b1 - b2
	return (((512 + rmean) * dr * dr) >> 8) + 4*dg*dg + (((767 - rmean) * db * db) >> 8)
}

// Color cube values for 6×6×6 palette (indices 16-231)
var cubeValues = [6]int{0, 95, 135, 175, 215, 255}

// RGBTo256 converts RGB to nearest 256-color palette index
// O(1) lookup via pre-computed Redmean LUT
func RGBTo256(c RGB) uint8 {
	return lut256[int(c.R>>2)<<12|int(c.G>>2)<<6|int(c.B>>2)]
}
