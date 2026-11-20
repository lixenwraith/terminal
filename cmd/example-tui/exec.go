//go:build !js

package main

import (
	"os"
	"syscall"
)

func execShell(sh string) { _ = syscall.Exec(sh, []string{sh}, os.Environ()) }
