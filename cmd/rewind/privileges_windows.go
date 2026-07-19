//go:build windows

package main

import "fmt"

func dropRootAgentPrivileges() error {
	return fmt.Errorf("native Windows process boundary is unavailable; refusing protected agent")
}
