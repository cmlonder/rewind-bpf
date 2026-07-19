//go:build !windows

package cgroup

import (
	"errors"
	"syscall"
)

func killPID(pid int) error        { return syscall.Kill(pid, syscall.SIGKILL) }
func isProcessGone(err error) bool { return errors.Is(err, syscall.ESRCH) }
