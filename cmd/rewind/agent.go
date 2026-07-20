package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rewindbpf/rewind/internal/agent"
)

func handleAgent(args []string) {
	if len(args) != 1 || args[0] != "list" {
		fatal("usage: rewind agent list")
	}
	if err := json.NewEncoder(os.Stdout).Encode(agent.List()); err != nil {
		fatal(fmt.Sprintf("encode agent adapters: %v", err))
	}
}
