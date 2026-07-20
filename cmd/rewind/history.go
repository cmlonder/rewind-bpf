package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/rewindbpf/rewind/internal/history"
)

func handleHistory(args []string) {
	if len(args) < 1 || args[0] != "prune" {
		fatal("usage: rewind history prune --path PATH --keep N")
	}
	flags := flag.NewFlagSet("history prune", flag.ContinueOnError)
	path := flags.String("path", "", "history JSON path")
	keep := flags.Int("keep", 30, "number of newest entries to retain")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*path) == "" || *keep < 0 {
		fatal("usage: rewind history prune --path PATH --keep N")
	}
	removed, err := history.Open(*path).PruneKeepLatest(*keep)
	if err != nil {
		fatal(fmt.Sprintf("prune history: %v", err))
	}
	fmt.Printf("history pruned: removed=%d keep=%d path=%s\n", removed, *keep, *path)
}
