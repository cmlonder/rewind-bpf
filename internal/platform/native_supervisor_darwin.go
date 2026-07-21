//go:build darwin

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ApplyNativeSupervisorAction reuses the same conflict-checked native
// transaction operations as the macOS CLI. The supervisor never accepts an
// arbitrary workspace path from the browser; it resolves the path from the
// persisted, authenticated run record.
func ApplyNativeSupervisorAction(ctx context.Context, action, recordPath, confirmation string) (NativeActionResult, error) {
	record, err := ReadNativeRecord(recordPath)
	if err != nil {
		return NativeActionResult{}, err
	}
	switch action {
	case "status":
		return NativeActionResult{State: record.State, Message: record.RunID}, nil
	case "rollback", "recover":
		if record.State == "committed" {
			return NativeActionResult{}, fmt.Errorf("native rollback cannot undo a committed destination")
		}
		if err := DiscardMacOSRuntime(record.RuntimeRoot, record.Workspace); err != nil {
			return NativeActionResult{}, err
		}
		if err := appendNativeSupervisorEvent(record.EventsPath, NativeEvent{Operation: action, Decision: "rollback", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			return NativeActionResult{}, err
		}
		record.State = "rolled_back"
		if err := WriteNativeRecord(recordPath, record); err != nil {
			return NativeActionResult{}, err
		}
		if err := PersistNativeHistoryAt(recordPath, record); err != nil {
			return NativeActionResult{}, err
		}
		return NativeActionResult{State: record.State, Message: "temporary native layer discarded"}, nil
	case "commit":
		if confirmation != "COMMIT" {
			return NativeActionResult{}, fmt.Errorf("commit requires confirmation=COMMIT")
		}
		if record.State != "succeeded" {
			return NativeActionResult{}, fmt.Errorf("native commit requires a succeeded review run, got %s", record.State)
		}
		if err := appendNativeSupervisorEvent(record.EventsPath, NativeEvent{Operation: "commit_attempt", Decision: "pending", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			return NativeActionResult{}, err
		}
		changes, err := AcceptMacOSRuntime(ctx, record.Workspace, record.RuntimeRoot, record.BaseManifest)
		if err != nil {
			return NativeActionResult{}, err
		}
		if err := appendNativeSupervisorEvent(record.EventsPath, NativeEvent{Operation: "commit", Decision: "allow", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			return NativeActionResult{}, err
		}
		record.Changes = changes
		record.State = "committed"
		if err := WriteNativeRecord(recordPath, record); err != nil {
			return NativeActionResult{}, err
		}
		if err := PersistNativeHistoryAt(recordPath, record); err != nil {
			return NativeActionResult{}, err
		}
		return NativeActionResult{State: record.State, Message: fmt.Sprintf("committed %d changes", len(changes))}, nil
	default:
		return NativeActionResult{}, fmt.Errorf("unsupported native supervisor action %q", action)
	}
}

func appendNativeSupervisorEvent(path string, event NativeEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}
