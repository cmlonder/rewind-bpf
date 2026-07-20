package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rewindbpf/rewind/internal/checkpoint"
)

func handleCheckpoint(args []string) {
	if len(args) == 0 || args[0] != "graph" {
		fatal("usage: rewind checkpoint graph inspect|add|transition ...")
	}
	if len(args) < 2 {
		fatal("usage: rewind checkpoint graph inspect|add|transition ...")
	}
	switch args[1] {
	case "inspect":
		checkpointInspect(args[2:])
	case "add":
		checkpointAdd(args[2:])
	case "transition":
		checkpointTransition(args[2:])
	default:
		fatal("usage: rewind checkpoint graph inspect|add|transition ...")
	}
}

func checkpointInspect(args []string) {
	flags := flag.NewFlagSet("checkpoint graph inspect", flag.ContinueOnError)
	path := flags.String("path", "", "checkpoint graph JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*path) == "" {
		fatal("usage: rewind checkpoint graph inspect --path PATH")
	}
	graph, err := checkpoint.Open(*path).Snapshot()
	if err != nil {
		fatal(err.Error())
	}
	if err := json.NewEncoder(os.Stdout).Encode(graph); err != nil {
		fatal(fmt.Sprintf("encode checkpoint graph: %v", err))
	}
}

func checkpointAdd(args []string) {
	flags := flag.NewFlagSet("checkpoint graph add", flag.ContinueOnError)
	path := flags.String("path", "", "checkpoint graph JSON path")
	id := flags.String("id", "", "checkpoint node id")
	runID := flags.String("run-id", "", "protected run id")
	parents := flags.String("parents", "", "comma-separated parent node ids")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*path) == "" || strings.TrimSpace(*id) == "" || strings.TrimSpace(*runID) == "" {
		fatal("usage: rewind checkpoint graph add --path PATH --id ID --run-id RUN_ID [--parents ID,...]")
	}
	var parentIDs []string
	for _, value := range strings.Split(*parents, ",") {
		if strings.TrimSpace(value) != "" {
			parentIDs = append(parentIDs, strings.TrimSpace(value))
		}
	}
	if err := checkpoint.Open(*path).Add(checkpoint.Node{ID: *id, RunID: *runID, Parents: parentIDs}); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("checkpoint node added: id=%s run_id=%s\n", *id, *runID)
}

func checkpointTransition(args []string) {
	flags := flag.NewFlagSet("checkpoint graph transition", flag.ContinueOnError)
	path := flags.String("path", "", "checkpoint graph JSON path")
	id := flags.String("id", "", "checkpoint node id")
	state := flags.String("state", "", "pending, running, succeeded, rolled_back, or blocked")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*path) == "" || strings.TrimSpace(*id) == "" || strings.TrimSpace(*state) == "" {
		fatal("usage: rewind checkpoint graph transition --path PATH --id ID --state STATE")
	}
	if err := checkpoint.Open(*path).Transition(*id, checkpoint.State(*state)); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("checkpoint node transitioned: id=%s state=%s\n", *id, *state)
}
