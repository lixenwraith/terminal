package terminal

import (
	"sync"
	"sync/atomic"

	"github.com/lixenwraith/color"
)

// ColorMode indicates terminal color capability
type ColorMode uint8

const (
	ColorMode256       ColorMode = iota // xterm-256 palette
	ColorModeTrueColor                  // 24-bit RGB
)

// 6-bit quantized LUT for Redmean-based 256-color mapping
// 64×64×64 = 262,144 bytes, fits in L2 cache
const lut256Size = 64 * 64 * 64

// init() -> first-use build. Construction is ~63M Redmean evaluations
// and 256 KiB resident; truecolor sessions never read the table.
var (
	lut256Ptr  atomic.Pointer[[lut256Size]uint8]
	lut256Once sync.Once
)

// lut256 returns the palette LUT, building it on first use.
// Fast path is an acquire load plus a predictable branch, and inlines into RGBTo256.
func lut256() *[lut256Size]uint8 {
	if p := lut256Ptr.Load(); p != nil {
		return p
	}
	return lut256Build()
}

// lut256Build populates the table. Cold path, kept out of line so lut256 stays inlinable.
// The release store publishes the fully written array to acquire loads in lut256.
//
//go:noinline
func lut256Build() *[lut256Size]uint8 {
	lut256Once.Do(func() {
		t := new([lut256Size]uint8)
		for r := range 64 {
			for g := range 64 {
				for b := range 64 {
					// Expand 6-bit to 8-bit (shift left 2, add 2 for midpoint)
					c := color.RGB{
						R: uint8(r<<2 | 2),
						G: uint8(g<<2 | 2),
						B: uint8(b<<2 | 2),
					}
					t[r<<12|g<<6|b] = computeRedmean256(c)
				}
			}
		}
		lut256Ptr.Store(t)
	})
	return lut256Ptr.Load()
}

// WarmPalette256 forces LUT construction. Idempotent, safe for concurrent use.
// Call before the first render when ColorMode is ColorMode256.
// RGBTo256 runs on the render path under the terminal mutex; a lazy build there stalls the frame.
func WarmPalette256() { _ = lut256() }

// computeRedmean256 finds the nearest 256-palette index using Redmean distance called from lut256Build, not init()
func computeRedmean256(c color.RGB) uint8 {
	// Grayscale fast path
	if c.R == c.G && c.G == c.B {
		if c.R < 8 {
			return 16
		}
		if c.R > 238 {
			return 231
		}
		return uint8(232 + (int(c.R)-8)/10)
	}

	bestIdx := uint8(16)
	minDist := 1 << 30

	// Search 6×6×6 cube (indices 16-231)
	for i := range 216 {
		cand := color.RGB{
			R: cubeValues[i/36],
			G: cubeValues[(i/6)%6],
			B: cubeValues[i%6],
		}
		if d := color.RedmeanDistance(c, cand); d < minDist {
			minDist = d
			bestIdx = uint8(16 + i)
		}
	}

	// Search grayscale ramp (indices 232-255)
	for i := range 24 {
		g := uint8(8 + i*10)
		if d := color.RedmeanDistance(c, color.RGB{R: g, G: g, B: g}); d < minDist {
			minDist = d
			bestIdx = uint8(232 + i)
		}
	}

	return bestIdx
}

// Color cube values for 6×6×6 palette (indices 16-231)
var cubeValues = [6]uint8{0, 95, 135, 175, 215, 255}

// RGBTo256 converts RGB to nearest 256-color palette index.
// O(1) via the Redmean LUT; the first call builds it (see WarmPalette256).
func RGBTo256(c color.RGB) uint8 {
	return lut256()[int(c.R>>2)<<12|int(c.G>>2)<<6|int(c.B>>2)]
}
