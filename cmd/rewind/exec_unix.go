//go:build !windows

package main

import (
	"syscall"
)

func execAgent(path string, args, env []string) error { return syscall.Exec(path, args, env) }
