package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

	path, err := exec.LookPath(command[0])
	if err != nil {
		fatal(fmt.Sprintf("find agent command: %v", err))
	}
	if err := syscall.Exec(path, command, os.Environ()); err != nil {
		fatal(fmt.Sprintf("exec agent command: %v", err))
	}
}

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
