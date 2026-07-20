//go:build !linux

package seccomp

import "fmt"

func InstallDenyRawSockets() error {
	return fmt.Errorf("raw-socket seccomp filter is Linux-only")
}

func InstallDenyNetwork() error {
	return fmt.Errorf("deny-network seccomp filter is Linux-only")
}
