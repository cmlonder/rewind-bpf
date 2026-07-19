//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func dropRootAgentPrivileges() error {
	if os.Geteuid() != 0 {
		return nil
	}
	uid, err := parseIDEnv("SUDO_UID")
	if err != nil {
		return fmt.Errorf("helper refuses root agent: %w", err)
	}
	gid, err := parseIDEnv("SUDO_GID")
	if err != nil {
		return fmt.Errorf("helper refuses root agent: %w", err)
	}
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("helper drop supplementary groups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("helper drop gid: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("helper drop uid: %w", err)
	}
	if os.Geteuid() == 0 {
		return fmt.Errorf("helper failed to drop root agent privileges")
	}
	return nil
}
