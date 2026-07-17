package main

import (
	"fmt"
	"os"
)

const usage = `RewindBPF — AI Agent Safety Runtime

Usage:
  rewind run [options] -- <agent-command>
  rewind status
  rewind events <run_id>
  rewind rollback <run_id>
  rewind commit <run_id>
  rewind policy check <policy.yaml>

This is the bootstrap CLI. Kernel and daemon integration will be added in the MVP build.
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Print(usage)
		return
	}

	switch os.Args[1] {
	case "run", "status", "events", "rollback", "commit", "policy":
		fmt.Printf("rewind: command %q is scaffolded; implementation is planned in docs/PROJECT_PLAN.md\n", os.Args[1])
	default:
		fmt.Fprintf(os.Stderr, "rewind: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
