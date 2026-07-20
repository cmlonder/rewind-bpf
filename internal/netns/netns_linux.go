//go:build linux

// Package netns owns the deliberately small Linux network-namespace boundary.
// It creates an isolated namespace but does not configure interfaces or
// routes, so the default posture is no egress. Allow-listed connectivity must
// be provided by a separate, reviewed broker rather than inferred here.
package netns

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func Enter() error {
	if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("enter isolated network namespace: %w", err)
	}
	return nil
}
