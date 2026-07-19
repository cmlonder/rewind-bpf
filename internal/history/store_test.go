package history

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertAndPruneKeepLatest(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	old := time.Unix(1, 0)
	newer := time.Unix(2, 0)
	if err := store.Upsert(Entry{RunID: "old", UpdatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(Entry{RunID: "new", UpdatedAt: newer}); err != nil {
		t.Fatal(err)
	}
	removed, err := store.PruneKeepLatest(1)
	if err != nil || removed != 1 {
		t.Fatalf("prune: removed=%d err=%v", removed, err)
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].RunID != "new" {
		t.Fatalf("entries: %+v err=%v", entries, err)
	}
}

func TestUpsertReplacesExistingRun(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	if err := store.Upsert(Entry{RunID: "run", State: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(Entry{RunID: "run", State: "rolled_back"}); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].State != "rolled_back" {
		t.Fatalf("entries: %+v err=%v", entries, err)
	}
}
