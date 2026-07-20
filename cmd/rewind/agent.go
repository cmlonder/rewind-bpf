package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rewindbpf/rewind/internal/agent"
)

func handleAgent(args []string) {
	if len(args) == 1 && args[0] == "list" {
		if err := json.NewEncoder(os.Stdout).Encode(agent.List()); err != nil {
			fatal(fmt.Sprintf("encode agent adapters: %v", err))
		}
		return
	}
	if len(args) != 2 || args[0] != "contract" {
		fatal("usage: rewind agent list | rewind agent contract generic|codex|openhands|claude-code")
	}
	spec, err := agent.Resolve(args[1])
	if err != nil {
		fatal(err.Error())
	}
	if err := json.NewEncoder(os.Stdout).Encode(spec); err != nil {
		fatal(fmt.Sprintf("encode agent contract: %v", err))
	}
}
