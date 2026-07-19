package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/event"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/runplan"
	"github.com/rewindbpf/rewind/internal/runstore"
	"github.com/rewindbpf/rewind/internal/telemetry"
)

func TestVerifyEvidenceChecksDigestAndChain(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	chain := telemetry.Chain{}
	value, err := chain.Append(event.Event{RunID: "run_test", PID: 1, Operation: event.Execve, TimestampNS: 1, Decision: event.Allow, Risk: event.Low})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(value); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	evidence, err := runstore.SummarizeEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	record := runstore.Record{Plan: runplan.Plan{Run: lifecycle.Run{ID: "run_test"}}, EventsPath: path, Events: evidence}
	result, err := Verify(record)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || !result.ChainValid || !result.MatchesRecord {
		t.Fatalf("unexpected verification result: %+v", result)
	}
}

func TestVerifyEvidenceRejectsTruncatedRecord(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	record := runstore.Record{Plan: runplan.Plan{Run: lifecycle.Run{ID: "run_test"}}, EventsPath: path, Events: runstore.EventEvidence{Complete: false, Truncated: true}}
	result, err := Verify(record)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || !result.Truncated {
		t.Fatalf("truncated evidence should fail: %+v", result)
	}
}
