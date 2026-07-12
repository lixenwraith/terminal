package terminal

import "math"

const softLightLUTSize = 256

// Perez SoftLight lookup tables (array access, no pointers)
// Pre-computed at init to avoid sqrt/division in per-cell loops
var (
	softLightG  [softLightLUTSize]float64
	softLightDF [softLightLUTSize]float64
)

func init() {
	for i := range softLightLUTSize {
		df := float64(i) / 255.0
		softLightDF[i] = df
		if df <= 0.25 {
			softLightG[i] = ((16.0*df-12.0)*df + 4.0) * df
		} else {
			softLightG[i] = math.Sqrt(df)
		}
	}
}

// clampU8 converts float to uint8 with saturation
func clampU8(v float64) uint8 {
	if v >= 255.0 {
		return 255
	}
	if v <= 0.0 {
		return 0
	}
	// Round-half-up; unifies rounding across Blend/Scale/Lerp/SoftLight
	return uint8(v + 0.5)
}

// addU8 is saturating uint8 addition
func addU8(a, b uint8) uint8 {
	sum := int(a) + int(b)
	if sum > 255 {
		return 255
	}
	return uint8(sum)
}

// fastDiv255 approximates x / 255 using integer math: (x + (x >> 8) + 1) >> 8
// Faster than DIV instruction, exact for x in [0, 255*255]
func fastDiv255(x int) int {
	return (x + (x >> 8) + 1) >> 8
}

// softLightChannel applies Perez soft light to one channel via LUTs
func softLightChannel(d, s uint8, intensity float64) uint8 {
	df := softLightDF[d]
	sf := softLightDF[s]

	var result float64
	if sf < 0.5 {
		result = df - (1.0-2.0*sf)*df*(1.0-df)
	} else {
		// LUT replaces math.Sqrt
		result = df + (2.0*sf-1.0)*(softLightG[d]-df)
	}

	// Lerp toward result by intensity, single dependency chain
	result = df + (result-df)*intensity

	return clampU8(result * 255.0)
}

// overlayChannel combines multiply (d < 128) and screen (d >= 128),
// preserving destination highlights and shadows
func overlayChannel(d, s uint8) uint8 {
	if d < 128 {
		return uint8(fastDiv255(2 * int(d) * int(s)))
	}
	return uint8(255 - fastDiv255(2*(255-int(d))*(255-int(s))))
}

// Blend performs linear alpha blend of src over dst
// alpha <= 0 returns dst, alpha >= 1 returns src
func Blend(dst, src RGB, alpha float64) RGB {
	if alpha >= 1.0 {
		return src
	}
	if alpha <= 0.0 {
		return dst
	}
	inv := 1.0 - alpha
	return RGB{
		R: clampU8(float64(src.R)*alpha + float64(dst.R)*inv),
		G: clampU8(float64(src.G)*alpha + float64(dst.G)*inv),
		B: clampU8(float64(src.B)*alpha + float64(dst.B)*inv),
	}
}

// SoftLight applies Perez soft light blend, gentler than linear alpha
// intensity in [0,1] mixes between dst and the blended result
func SoftLight(dst, src RGB, intensity float64) RGB {
	return RGB{
		R: softLightChannel(dst.R, src.R, intensity),
		G: softLightChannel(dst.G, src.G, intensity),
		B: softLightChannel(dst.B, src.B, intensity),
	}
}

// Max returns per-channel maximum, alpha-blended over dst
func Max(dst, src RGB, alpha float64) RGB {
	if alpha <= 0.0 {
		return dst
	}
	maxed := RGB{
		R: max(dst.R, src.R),
		G: max(dst.G, src.G),
		B: max(dst.B, src.B),
	}
	if alpha >= 1.0 {
		return maxed
	}
	return Blend(dst, maxed, alpha)
}

// Add performs saturating additive blend, alpha-blended over dst
func Add(dst, src RGB, alpha float64) RGB {
	if alpha <= 0.0 {
		return dst
	}
	added := RGB{
		R: addU8(dst.R, src.R),
		G: addU8(dst.G, src.G),
		B: addU8(dst.B, src.B),
	}
	if alpha >= 1.0 {
		return added
	}
	return Blend(dst, added, alpha)
}

// Screen applies 1-(1-dst)*(1-src), alpha-blended over dst
// Always lightens; useful for glow accumulation without clipping harshness of Add
func Screen(dst, src RGB, alpha float64) RGB {
	if alpha <= 0.0 {
		return dst
	}
	screened := RGB{
		R: uint8(255 - fastDiv255((255-int(dst.R))*(255-int(src.R)))),
		G: uint8(255 - fastDiv255((255-int(dst.G))*(255-int(src.G)))),
		B: uint8(255 - fastDiv255((255-int(dst.B))*(255-int(src.B)))),
	}
	if alpha >= 1.0 {
		return screened
	}
	return Blend(dst, screened, alpha)
}

// Overlay combines multiply (darks) and screen (lights), alpha-blended over dst
func Overlay(dst, src RGB, alpha float64) RGB {
	if alpha <= 0.0 {
		return dst
	}
	overlaid := RGB{
		R: overlayChannel(dst.R, src.R),
		G: overlayChannel(dst.G, src.G),
		B: overlayChannel(dst.B, src.B),
	}
	if alpha >= 1.0 {
		return overlaid
	}
	return Blend(dst, overlaid, alpha)
}

// Scale multiplies all channels by factor, saturating (factor > 1.0 brightens)
func Scale(c RGB, factor float64) RGB {
	return RGB{
		R: clampU8(float64(c.R) * factor),
		G: clampU8(float64(c.G) * factor),
		B: clampU8(float64(c.B) * factor),
	}
}

// Grayscale converts to grayscale using Rec. 601 luma coefficients
// Y = R*0.299 + G*0.587 + B*0.114, integer math
func Grayscale(c RGB) RGB {
	gray := uint8((int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000)
	return RGB{R: gray, G: gray, B: gray}
}
