//go:build !linux

package terminal

// restoreConsoleKeys has no console keyboard to restore here
func restoreConsoleKeys() {}
