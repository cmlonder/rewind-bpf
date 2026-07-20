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
	if len(args) == 0 {
		fatal("usage: rewind bundle create --record PATH --output PATH | rewind bundle verify --input PATH")
	}
	if args[0] == "verify" {
		handleBundleVerify(args[1:])
		return
	}
	if args[0] != "create" {
		fatal("usage: rewind bundle create --record PATH --output PATH | rewind bundle verify --input PATH")
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

func handleBundleVerify(args []string) {
	flags := flag.NewFlagSet("bundle verify", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence .tar.gz path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" {
		fatal("usage: rewind bundle verify --input PATH")
	}
	metadata, err := evidencebundle.Verify(*inputPath)
	if err != nil {
		fatal(fmt.Sprintf("evidence bundle verification failed: %v", err))
	}
	fmt.Printf("evidence bundle verified: run_id=%s artifacts=%d state=%s\n", metadata.RunID, len(metadata.Artifacts), metadata.State)
}
