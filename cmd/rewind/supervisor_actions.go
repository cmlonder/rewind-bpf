package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/rewindbpf/rewind/internal/acceptance"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/protectedrun"
	"github.com/rewindbpf/rewind/internal/runstore"
	"github.com/rewindbpf/rewind/internal/supervisor"
)

// supervisorAction is the privileged boundary for the local control plane.
// It deliberately accepts only actions that already have equivalent, audited
// CLI paths; arbitrary commands and policy mutation never cross this boundary.
func supervisorAction(historyPath string, request supervisor.Request) (supervisor.Response, error) {
	if strings.TrimSpace(request.RunID) == "" {
		return supervisor.Response{}, fmt.Errorf("run_id is required")
	}
	store := history.Open(historyPath)
	entry, err := historyEntry(store, request.RunID)
	if err != nil {
		return supervisor.Response{}, err
	}
	record, err := runstore.Read(entry.RecordPath)
	if err != nil {
		return supervisor.Response{}, fmt.Errorf("read run record: %w", err)
	}

	switch request.Action {
	case "status":
		return supervisor.Response{OK: true, State: string(record.Plan.Run.State), Message: record.Plan.Run.ID}, nil
	case "rollback", "recover":
		return supervisorRollback(entry.RecordPath, record)
	case "commit":
		if request.Confirmation != "COMMIT" {
			return supervisor.Response{}, fmt.Errorf("commit requires confirmation=COMMIT")
		}
		return supervisorCommit(entry.RecordPath, record)
	default:
		return supervisor.Response{}, fmt.Errorf("unsupported supervisor action %q", request.Action)
	}
}

func historyEntry(store history.Store, runID string) (history.Entry, error) {
	entries, err := store.List()
	if err != nil {
		return history.Entry{}, err
	}
	for _, entry := range entries {
		if entry.RunID == runID {
			return entry, nil
		}
	}
	return history.Entry{}, fmt.Errorf("run %s not found in history", runID)
}

func supervisorRollback(recordPath string, record runstore.Record) (supervisor.Response, error) {
	coordinator := protectedrun.Coordinator{
		Overlay: overlay.Manager{Backend: record.Plan.OverlayBackend},
		Scope:   persistedScope(record.Plan.CgroupPath),
	}
	if err := coordinator.RollbackPlan(context.Background(), &record.Plan); err != nil {
		return supervisor.Response{}, err
	}
	if err := persistRecordState(recordPath, record.Plan, record.EventsPath, evidenceState{dropped: record.Events.Dropped, truncated: record.Events.Truncated}); err != nil {
		return supervisor.Response{}, err
	}
	if err := persistHistory(record.Plan.HistoryPath, record.Plan, recordPath); err != nil {
		return supervisor.Response{}, err
	}
	return supervisor.Response{OK: true, State: string(record.Plan.Run.State), Message: "temporary layer discarded"}, nil
}

func supervisorCommit(recordPath string, record runstore.Record) (supervisor.Response, error) {
	if record.Plan.Run.State != lifecycle.Succeeded {
		return supervisor.Response{}, fmt.Errorf("commit requires a succeeded review run, got %s", record.Plan.Run.State)
	}
	if err := evidenceCompleteness(record); err != nil {
		return supervisor.Response{}, err
	}
	candidate, err := manifest.Build(record.Plan.Layout.Merged)
	if err != nil {
		return supervisor.Response{}, fmt.Errorf("build candidate manifest: %w", err)
	}
	destination, err := manifest.Build(record.Plan.Layout.Lower)
	if err != nil {
		return supervisor.Response{}, fmt.Errorf("build destination manifest: %w", err)
	}
	report, err := acceptance.Apply(record.Plan.Manifest, destination, candidate, record.Plan.Layout.Merged, record.Plan.Layout.Lower)
	if err != nil {
		return supervisor.Response{}, fmt.Errorf("commit refused: %w", err)
	}
	if err := (overlay.Manager{Backend: record.Plan.OverlayBackend}).Rollback(context.Background(), record.Plan.Layout); err != nil {
		return supervisor.Response{}, fmt.Errorf("commit cleanup: %w", err)
	}
	if err := record.Plan.Run.Transition(lifecycle.Committed); err != nil {
		return supervisor.Response{}, err
	}
	if err := runstore.Write(recordPath, record); err != nil {
		return supervisor.Response{}, err
	}
	if err := persistHistory(record.Plan.HistoryPath, record.Plan, recordPath); err != nil {
		return supervisor.Response{}, err
	}
	return supervisor.Response{OK: true, State: string(record.Plan.Run.State), Message: fmt.Sprintf("committed %d changes", len(report.Changes))}, nil
}
