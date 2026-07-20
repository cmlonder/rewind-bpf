//go:build !linux

package seccomp

import "fmt"

func InstallDenyRawSockets() error {
	return fmt.Errorf("raw-socket seccomp filter is Linux-only")
}
