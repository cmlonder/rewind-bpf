package platform

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/rewindbpf/rewind/internal/history"
)

// NativeActionResult is the platform-neutral result returned by a local
// supervisor when it operates on a native transaction record.
type NativeActionResult struct {
	State   string
	Message string
}

// NativeRecordForSupervisor distinguishes native records from the Linux
// runstore contract without trusting a filename or platform supplied by the
// browser. The record version and run id must both validate.
func NativeRecordForSupervisor(path, runID string) (NativeRecord, bool, error) {
	record, err := ReadNativeRecord(path)
	if err != nil {
		return NativeRecord{}, false, nil
	}
	if runID != "" && record.RunID != runID {
		return NativeRecord{}, false, fmt.Errorf("native record run id mismatch: want %s got %s", runID, record.RunID)
	}
	return record, true, nil
}

// PersistNativeHistoryAt is the normal entry point because NativeRecord does
// not embed the record filename. It is kept separate to avoid changing the
// portable record schema solely for the index.
func PersistNativeHistoryAt(recordPath string, record NativeRecord) error {
	if record.HistoryPath == "" {
		return nil
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	return history.Open(record.HistoryPath).Upsert(history.Entry{
		RunID:      record.RunID,
		Backend:    record.Backend,
		State:      record.State,
		Workspace:  record.Workspace,
		RecordPath: filepath.Clean(recordPath),
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	})
}
