//go:build !linux

package netns

import "fmt"

func Enter() error { return fmt.Errorf("network namespaces are Linux-only") }
