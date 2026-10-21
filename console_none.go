//go:build !linux && !freebsd

package terminal

// restoreConsoleKeys has no console keyboard to restore here
func restoreConsoleKeys() {}
