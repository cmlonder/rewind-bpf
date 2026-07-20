package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rewindbpf/rewind/internal/acceptance"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func handleCommit(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind commit is Linux-only; use the disposable Ubuntu VM")
	}
	flags := flag.NewFlagSet("rewind commit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	confirm := flags.Bool("confirm", false, "explicitly apply the reviewed candidate to the original workspace")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind commit --record PATH --confirm")
	}
	if !*confirm {
		fatal("commit is destructive to the destination; pass --confirm after reviewing diff/export")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if record.Plan.Run.State != lifecycle.Succeeded {
		fatal(fmt.Sprintf("commit requires a succeeded review run, got %s", record.Plan.Run.State))
	}
	if err := evidenceCompleteness(record); err != nil {
		fatal(err.Error())
	}
	candidate, err := manifest.Build(record.Plan.Layout.Merged)
	if err != nil {
		fatal(fmt.Sprintf("build candidate manifest: %v", err))
	}
	destination, err := manifest.Build(record.Plan.Layout.Lower)
	if err != nil {
		fatal(fmt.Sprintf("build destination manifest: %v", err))
	}
	report, err := acceptance.Apply(record.Plan.Manifest, destination, candidate, record.Plan.Layout.Merged, record.Plan.Layout.Lower)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		fatal(fmt.Sprintf("commit refused: %v", err))
	}
	// A review run keeps the merged mount alive. Apply the candidate first,
	// then unmount and discard the temporary layer before recording committed
	// state so the destination remains the only durable copy.
	if err := (overlay.Manager{Backend: record.Plan.OverlayBackend}).Rollback(context.Background(), record.Plan.Layout); err != nil {
		fatal(fmt.Sprintf("commit cleanup: %v", err))
	}
	if err := record.Plan.Run.Transition(lifecycle.Committed); err != nil {
		fatal(err.Error())
	}
	if err := runstore.Write(*recordPath, record); err != nil {
		fatal(err.Error())
	}
	if err := persistHistory(record.Plan.HistoryPath, record.Plan, *recordPath); err != nil {
		fatal(fmt.Sprintf("persist commit history: %v", err))
	}
	fmt.Printf("run committed: run_id=%s changes=%d record=%s\n", record.Plan.Run.ID, len(report.Changes), *recordPath)
}

func evidenceCompleteness(record runstore.Record) error {
	if record.Events.Dropped > 0 || record.Events.Truncated || !record.Events.Complete {
		return fmt.Errorf("commit refused: evidence is incomplete (dropped=%d truncated=%t complete=%t)", record.Events.Dropped, record.Events.Truncated, record.Events.Complete)
	}
	return nil
}
