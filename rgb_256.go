package terminal

// Generic xterm 256-color palette indices without game semantics
// Game systems reference these via aliases in their own parameter files
//
// Color cube: index = 16 + 36*r + 6*g + b where r,g,b ∈ [0,5]
// Grayscale ramp: indices 232-255, level = 8 + 10*(index-232)
//
// Ordered dark-to-light within each hue group

const (
	// --- Blue ---
	P256DeepNavy  uint8 = 17 // (0,0,1)
	P256DarkBlue  uint8 = 18 // (0,0,2)
	P256SteelBlue uint8 = 75 // (1,3,5)
	P256LightBlue uint8 = 81 // (1,4,5)

	// --- Teal / Cyan ---
	P256DeepTeal  uint8 = 23 // (0,1,1)
	P256Teal      uint8 = 44 // (0,4,4)
	P256Green     uint8 = 46 // (0,5,0)
	P256Cyan      uint8 = 51 // (0,5,5)
	P256LightCyan uint8 = 87 // (1,5,5)

	// --- Blue / Purple ---
	P256CobaltBlue     uint8 = 33  // (0,2,5)
	P256DarkPurpleBlue uint8 = 54  // (1,0,2)
	P256Indigo         uint8 = 63  // (1,1,5)
	P256Purple         uint8 = 129 // (3,0,5)
	P256Violet         uint8 = 134 // (3,1,4)
	P256MediumPurple   uint8 = 135 // (3,1,5)
	P256Orchid         uint8 = 176 // (4,2,4)

	// --- Green / Yellow-Green ---
	P256YellowGreen uint8 = 154 // (3,5,0)

	// --- Red ---
	P256Maroon      uint8 = 52  // (1,0,0)
	P256DarkCrimson uint8 = 88  // (2,0,0)
	P256Crimson     uint8 = 160 // (4,0,0)

	// --- Red / Orange / Yellow ---
	P256Red       uint8 = 196 // (5,0,0)
	P256Rose      uint8 = 198 // (5,0,2)
	P256RedOrange uint8 = 202 // (5,1,0)
	P256Orange    uint8 = 208 // (5,2,0)
	P256Amber     uint8 = 214 // (5,3,0)
	P256Gold      uint8 = 220 // (5,4,0)
	P256Yellow    uint8 = 226 // (5,5,0)

	// --- Orange / Brown ---
	P256DarkAmber uint8 = 94 // (2,1,0)

	// --- Grayscale ---
	P256Gray uint8 = 240 // Grayscale step 8, level ~88
)

// Cube256 returns the xterm 256-palette index for an RGB cube coordinate.
// r, g, b must be in [0,5]. Values outside that range are clamped.
func Cube256(r, g, b uint8) uint8 {
	if r > 5 {
		r = 5
	}
	if g > 5 {
		g = 5
	}
	if b > 5 {
		b = 5
	}
	return 16 + 36*r + 6*g + b
}

// CubeRGB256 returns the (r, g, b) cube coordinates for a 256-palette color cube index.
// Index must be in [16,231]. Returns (0,0,0) for out-of-range indices.
func CubeRGB256(index uint8) (r, g, b uint8) {
	if index < 16 || index > 231 {
		return 0, 0, 0
	}
	n := index - 16
	r = n / 36
	g = (n % 36) / 6
	b = n % 6
	return r, g, b
}

// Gray256 returns the xterm 256-palette index for a grayscale step.
// step must be in [0,23] (maps to indices 232-255, levels 8-238).
func Gray256(step uint8) uint8 {
	if step > 23 {
		step = 23
	}
	return 232 + step
}