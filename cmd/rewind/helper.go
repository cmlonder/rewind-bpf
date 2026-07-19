package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

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

	var plan *landlock.Plan
	if *planPath != "" {
		file, err := os.Open(*planPath)
		if err != nil {
			fatal(fmt.Sprintf("open Landlock plan: %v", err))
		}
		var value landlock.Plan
		decodeErr := json.NewDecoder(file).Decode(&value)
		closeErr := file.Close()
		if decodeErr != nil {
			fatal(fmt.Sprintf("decode Landlock plan: %v", decodeErr))
		}
		if closeErr != nil {
			fatal(fmt.Sprintf("close Landlock plan: %v", closeErr))
		}
		plan = &value
	}
	if err := dropRootAgentPrivileges(); err != nil {
		fatal(err.Error())
	}
	if plan != nil {
		if err := landlock.Apply(*plan); err != nil {
			fatal(err.Error())
		}
	}
	if err := waitForStartGate(); err != nil {
		fatal(err.Error())
	}

	path, err := exec.LookPath(command[0])
	if err != nil {
		fatal(fmt.Sprintf("find agent command: %v", err))
	}
	if err := execAgent(path, command, os.Environ()); err != nil {
		fatal(fmt.Sprintf("exec agent command: %v", err))
	}
}

func waitForStartGate() error {
	value := os.Getenv("REWIND_START_GATE_FD")
	if value == "" {
		return nil
	}
	fd, err := strconv.Atoi(value)
	if err != nil || fd < 3 {
		return fmt.Errorf("helper start gate fd is invalid")
	}
	file := os.NewFile(uintptr(fd), "rewind-start-gate")
	if file == nil {
		return fmt.Errorf("helper start gate fd is unavailable")
	}
	defer file.Close()
	var signal [1]byte
	if _, err := io.ReadFull(file, signal[:]); err != nil {
		return fmt.Errorf("helper wait for start gate: %w", err)
	}
	return nil
}

func parseIDEnv(name string) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return 0, fmt.Errorf("%s is missing", name)
	}
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%s is invalid", name)
	}
	return id, nil
}
