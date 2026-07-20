package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	evidencebundle "github.com/rewindbpf/rewind/internal/bundle"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func handleBundle(args []string) {
	if len(args) == 0 || args[0] != "create" {
		fatal("usage: rewind bundle create --record PATH --output PATH")
	}
	flags := flag.NewFlagSet("bundle create", flag.ContinueOnError)
	recordPath := flags.String("record", "", "run record JSON path")
	outputPath := flags.String("output", "", "evidence .tar.gz output path")
	if err := flags.Parse(args[1:]); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" || strings.TrimSpace(*outputPath) == "" {
		fatal("usage: rewind bundle create --record PATH --output PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	outputAbs, err := filepath.Abs(*outputPath)
	if err != nil {
		fatal(fmt.Sprintf("resolve bundle output: %v", err))
	}
	if pathWithin(record.Plan.Layout.Root, outputAbs) {
		fatal("bundle output must be outside the run runtime root")
	}
	metadata, err := evidencebundle.Create(outputAbs, *recordPath, record.Plan.Layout.Root, record)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("wrote evidence bundle: %s artifacts=%d run_id=%s\n", outputAbs, len(metadata.Artifacts), metadata.RunID)
}
