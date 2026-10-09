//go:build wasm

package terminal

import "os"

// DetectColorMode determines terminal color capability from environment
func DetectColorMode() ColorMode {
	// Browsers/xterm.js generally support true color
	return ColorModeTrueColor
}

// resetTerminalMode is no-op for WASM; termios does not exist
func resetTerminalMode() {}

// controllingTTY is nil: the browser terminal is reached through the bridge
func controllingTTY() *os.File { return nil }
