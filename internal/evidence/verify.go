// Package evidence verifies a persisted RewindBPF event stream independently
// from the protected-run coordinator. It only reads the record and JSONL log.
package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rewindbpf/rewind/internal/runstore"
	"github.com/rewindbpf/rewind/internal/telemetry"
)

type Result struct {
	RunID          string `json:"run_id"`
	Count          uint64 `json:"count"`
	Bytes          uint64 `json:"bytes"`
	Dropped        uint64 `json:"dropped"`
	Truncated      bool   `json:"truncated"`
	Complete       bool   `json:"complete"`
	StreamComplete bool   `json:"stream_complete"`
	RecordComplete bool   `json:"record_complete"`
	ChainValid     bool   `json:"chain_valid"`
	MatchesRecord  bool   `json:"matches_record"`
}

func Verify(record runstore.Record) (Result, error) {
	var journal []telemetry.JournalEvent
	for _, path := range runstore.EventLogPaths(record) {
		file, err := os.Open(path)
		if err != nil {
			return Result{}, fmt.Errorf("verify evidence: open events: %w", err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			var value telemetry.JournalEvent
			if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
				_ = file.Close()
				return Result{}, fmt.Errorf("verify evidence: decode event journal: %w", err)
			}
			journal = append(journal, value)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return Result{}, fmt.Errorf("verify evidence: read event journal: %w", err)
		}
		if err := file.Close(); err != nil {
			return Result{}, fmt.Errorf("verify evidence: close event journal: %w", err)
		}
	}
	evidence, err := runstore.SummarizeEventsPaths(runstore.EventLogPaths(record))
	if err != nil {
		return Result{}, err
	}
	chainValid := telemetry.Verify(journal)
	matchesRecord := evidence.Count == record.Events.Count && evidence.Bytes == record.Events.Bytes && evidence.SHA256 == record.Events.SHA256 && evidence.Complete == record.Events.Complete
	complete := evidence.Complete && record.Events.Complete && record.Events.Dropped == 0 && !record.Events.Truncated && chainValid && matchesRecord
	return Result{
		RunID:          record.Plan.Run.ID,
		Count:          evidence.Count,
		Bytes:          evidence.Bytes,
		Dropped:        record.Events.Dropped,
		Truncated:      record.Events.Truncated,
		Complete:       complete,
		StreamComplete: evidence.Complete,
		RecordComplete: record.Events.Complete,
		ChainValid:     chainValid,
		MatchesRecord:  matchesRecord,
	}, nil
}
