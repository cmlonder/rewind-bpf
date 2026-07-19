//go:build windows

package main

import (
	"os"
	"os/exec"
)

func execAgent(path string, args, env []string) error {
	command := exec.Command(path, args[1:]...)
	command.Args = args
	command.Env = env
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}
