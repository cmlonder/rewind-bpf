package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/rewindbpf/rewind/internal/landlock"
)

func handleHelper(args []string) {
	flags := flag.NewFlagSet("rewind helper", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	planPath := flags.String("plan-file", "", "JSON Landlock plan written by the parent runtime")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	command := flags.Args()
	if len(command) == 0 {
		fatal("usage: rewind helper [--plan-file PATH] -- <agent-command> [args...]")
	}

	if *planPath != "" {
		file, err := os.Open(*planPath)
		if err != nil {
			fatal(fmt.Sprintf("open Landlock plan: %v", err))
		}
		var plan landlock.Plan
		decodeErr := json.NewDecoder(file).Decode(&plan)
		closeErr := file.Close()
		if decodeErr != nil {
			fatal(fmt.Sprintf("decode Landlock plan: %v", decodeErr))
		}
		if closeErr != nil {
			fatal(fmt.Sprintf("close Landlock plan: %v", closeErr))
		}
		if err := landlock.Apply(plan); err != nil {
			fatal(err.Error())
		}
	}

	path, err := exec.LookPath(command[0])
	if err != nil {
		fatal(fmt.Sprintf("find agent command: %v", err))
	}
	if err := syscall.Exec(path, command, os.Environ()); err != nil {
		fatal(fmt.Sprintf("exec agent command: %v", err))
	}
}
