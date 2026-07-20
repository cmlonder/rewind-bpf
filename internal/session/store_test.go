package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLeaseAcquireHeartbeatTakeoverAndRelease(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "sessions.json"))
	clock := time.Unix(100, 0)
	lease, err := store.Apply(Request{Action: "acquire", RunID: "run-1", Owner: "browser-a", TTL: 60}, clock)
	if err != nil || lease.ID == "" {
		t.Fatalf("acquire=%+v err=%v", lease, err)
	}
	if _, err := store.Apply(Request{Action: "acquire", RunID: "run-1", Owner: "browser-b"}, clock); err == nil {
		t.Fatal("second owner acquired active lease")
	}
	heartbeat, err := store.Apply(Request{Action: "heartbeat", RunID: "run-1", Owner: "browser-a", TTL: 120}, clock.Add(30*time.Second))
	if err != nil || !heartbeat.ExpiresAt.Equal(clock.Add(150*time.Second)) {
		t.Fatalf("heartbeat=%+v err=%v", heartbeat, err)
	}
	takeover, err := store.Apply(Request{Action: "takeover", RunID: "run-1", Owner: "browser-b", TTL: 30}, clock.Add(2*time.Minute))
	if err != nil || takeover.Owner != "browser-b" {
		t.Fatalf("takeover=%+v err=%v", takeover, err)
	}
	if _, err := store.Apply(Request{Action: "release", RunID: "run-1", Owner: "browser-a"}, clock); err == nil {
		t.Fatal("old owner released takeover")
	}
	if _, err := store.Apply(Request{Action: "release", RunID: "run-1", Owner: "browser-b"}, clock); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List()
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
}
