package terminal

import "github.com/lixenwraith/color"

// Generic xterm 256-color constants proxied to the central color package
// to maintain backwards compatibility for the terminal package.

const (
	P256DeepNavy       = color.P256DeepNavy
	P256DarkBlue       = color.P256DarkBlue
	P256SteelBlue      = color.P256SteelBlue
	P256LightBlue      = color.P256LightBlue
	P256DeepTeal       = color.P256DeepTeal
	P256Teal           = color.P256Teal
	P256Green          = color.P256Green
	P256Cyan           = color.P256Cyan
	P256LightCyan      = color.P256LightCyan
	P256CobaltBlue     = color.P256CobaltBlue
	P256DarkPurpleBlue = color.P256DarkPurpleBlue
	P256Indigo         = color.P256Indigo
	P256Purple         = color.P256Purple
	P256Violet         = color.P256Violet
	P256MediumPurple   = color.P256MediumPurple
	P256Orchid         = color.P256Orchid
	P256YellowGreen    = color.P256YellowGreen
	P256Maroon         = color.P256Maroon
	P256DarkCrimson    = color.P256DarkCrimson
	P256Crimson        = color.P256Crimson
	P256Red            = color.P256Red
	P256Rose           = color.P256Rose
	P256RedOrange      = color.P256RedOrange
	P256Orange         = color.P256Orange
	P256Amber          = color.P256Amber
	P256Gold           = color.P256Gold
	P256Yellow         = color.P256Yellow
	P256DarkAmber      = color.P256DarkAmber
	P256Gray           = color.P256Gray
)

// Cube256 returns the xterm 256-palette index for an RGB cube coordinate.
func Cube256(r, g, b uint8) uint8 {
	return color.Cube256(r, g, b)
}

// CubeRGB256 returns the (r, g, b) cube coordinates for a 256-palette color cube index.
func CubeRGB256(index uint8) (r, g, b uint8) {
	return color.CubeRGB256(index)
}

// Gray256 returns the xterm 256-palette index for a grayscale step.
func Gray256(step uint8) uint8 {
	return color.Gray256(step)
}

