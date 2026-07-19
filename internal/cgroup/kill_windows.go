//go:build windows

package cgroup

import (
	"errors"
	"os"
)

// Windows has no POSIX cgroup-procs signal primitive. The future native
// supervisor will own the process job object; this compile-safe fallback kills
// the individual process and never claims descendant containment.
func killPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func isProcessGone(err error) bool { return errors.Is(err, os.ErrProcessDone) }
