//go:build !darwin

package main

func handleNative(args []string) {
	fatal("rewind native targets macOS; use the Linux protected run inside the disposable Ubuntu VM")
}
