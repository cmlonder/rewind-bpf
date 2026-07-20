package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rewindbpf/rewind/internal/branch"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func handleBranch(args []string) {
	if len(args) == 0 || args[0] != "apply" {
		fatal("usage: rewind branch apply --record PATH --repo PATH --branch NAME --confirm [--commit --message TEXT]")
	}
	if runtime.GOOS != "linux" {
		fatal("rewind branch apply is Linux-only because the source run uses the protected merged view")
	}
	flags := flag.NewFlagSet("branch apply", flag.ContinueOnError)
	recordPath := flags.String("record", "", "succeeded review run record JSON path")
	repoPath := flags.String("repo", "", "checked-out Git repository root")
	branchName := flags.String("branch", "", "exact checked-out branch name")
	confirm := flags.Bool("confirm", false, "explicitly apply the reviewed candidate to the branch")
	commit := flags.Bool("commit", false, "create a Git commit after applying")
	message := flags.String("message", "", "Git commit message")
	if err := flags.Parse(args[1:]); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" || strings.TrimSpace(*repoPath) == "" || strings.TrimSpace(*branchName) == "" || !*confirm {
		fatal("usage: rewind branch apply --record PATH --repo PATH --branch NAME --confirm [--commit --message TEXT]")
	}
	if *commit && strings.TrimSpace(*message) == "" {
		fatal("usage: --message is required with --commit")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if record.Plan.Run.State != lifecycle.Succeeded {
		fatal(fmt.Sprintf("branch apply requires a succeeded review run, got %s", record.Plan.Run.State))
	}
	if err := evidenceCompleteness(record); err != nil {
		fatal(err.Error())
	}
	candidate, err := manifest.Build(record.Plan.Layout.Merged)
	if err != nil {
		fatal(fmt.Sprintf("build branch candidate manifest: %v", err))
	}
	destination, err := manifest.Build(*repoPath)
	if err != nil {
		fatal(fmt.Sprintf("build branch destination manifest: %v", err))
	}
	report, err := branch.Apply(record.Plan.Manifest, destination, candidate, record.Plan.Layout.Merged, *repoPath, *branchName, *message, *commit)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		fatal(fmt.Sprintf("branch apply refused: %v", err))
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fatal(fmt.Sprintf("encode branch report: %v", err))
	}
}
