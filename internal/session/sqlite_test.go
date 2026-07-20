package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreCoordinatesLeases(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	lease, err := store.Apply(Request{Action: "acquire", RunID: "run-1", Owner: "alice", TTL: 30}, now)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Owner != "alice" {
		t.Fatalf("lease=%+v", lease)
	}
	if _, err := store.Apply(Request{Action: "acquire", RunID: "run-1", Owner: "bob", TTL: 30}, now); err == nil {
		t.Fatal("expected ownership refusal")
	}
	if _, err := store.Apply(Request{Action: "heartbeat", RunID: "run-1", Owner: "alice", TTL: 30}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(Request{Action: "release", RunID: "run-1", Owner: "alice"}, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	leases, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 0 {
		t.Fatalf("leases=%v", leases)
	}
}

func TestSQLiteStoreExpiresLeases(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	if _, err := store.Apply(Request{Action: "acquire", RunID: "run-1", Owner: "alice", TTL: 1}, now); err != nil {
		t.Fatal(err)
	}
	leases, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 {
		t.Fatal(leases)
	}
	if _, err := store.Apply(Request{Action: "takeover", RunID: "run-1", Owner: "bob", TTL: 30}, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	leases, err = store.List()
	if err != nil || len(leases) != 1 || leases[0].Owner != "bob" {
		t.Fatalf("leases=%v err=%v", leases, err)
	}
}
