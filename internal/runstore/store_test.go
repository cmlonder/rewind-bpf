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
