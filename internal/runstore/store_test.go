package runstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/runplan"
)

func TestWriteReadRecordIsAtomicAndPrivate(t *testing.T) {
	workspace := t.TempDir()
	plan, err := runplan.Build(runplan.Config{
		Workspace:   workspace,
		RuntimeRoot: filepath.Join(t.TempDir(), "run"),
		Policy:      policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeOff}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nested", "run.json")
	if err := Write(path, Record{Plan: plan, EventsPath: "/tmp/events.jsonl"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("record mode = %o, want 600", got)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.Run.ID != plan.Run.ID || got.EventsPath != "/tmp/events.jsonl" {
		t.Fatalf("record = %+v", got)
	}
}

func TestSummarizeEventsCountsBytesAndDetectsTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("{\"event\":1}\n{\"event\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := SummarizeEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 2 || summary.Bytes != 24 || !summary.Complete || summary.SHA256 == "" {
		t.Fatalf("summary = %+v", summary)
	}
	if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err = SummarizeEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Complete {
		t.Fatal("expected incomplete final line")
	}
}

func TestEventEvidenceWithDroppedMarksIncomplete(t *testing.T) {
	evidence := EventEvidence{Count: 4, Bytes: 100, SHA256: "digest", Complete: true}
	evidence = evidence.WithDropped(3)
	if evidence.Dropped != 3 {
		t.Fatalf("dropped = %d, want 3", evidence.Dropped)
	}
	if evidence.Complete {
		t.Fatal("evidence with dropped events must be incomplete")
	}
	zero := (EventEvidence{Count: 4, Bytes: 100, SHA256: "digest", Complete: true}).WithDropped(0)
	if zero.Dropped != 0 || !zero.Complete {
		t.Fatalf("zero dropped events should preserve complete evidence: %+v", zero)
	}
}
