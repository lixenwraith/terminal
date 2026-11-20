//go:build wasm

package terminal

import "os"

// DetectColorMode is truecolor: a page paints RGB
func DetectColorMode() ColorMode { return ColorModeTrueColor }

// resetTerminalMode has no termios to restore in a browser
func resetTerminalMode() {}

// controllingTTY is nil: a page is no tty
func controllingTTY() *os.File { return nil }
