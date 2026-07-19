package telemetry

import (
	"testing"

	"github.com/rewindbpf/rewind/internal/event"
)

func TestChainAppendsAndVerifies(t *testing.T) {
	chain := Chain{}
	first, err := chain.Append(event.Event{RunID: "run_test", PID: 1, Operation: event.Execve, TimestampNS: 1, Decision: event.Allow, Risk: event.Low})
	if err != nil {
		t.Fatal(err)
	}
	second, err := chain.Append(event.Event{RunID: "run_test", PID: 2, Operation: event.Write, TimestampNS: 2, Decision: event.Allow, Risk: event.High})
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || second.Sequence != 2 || second.PreviousHash != first.Hash {
		t.Fatalf("unexpected chain: first=%+v second=%+v", first, second)
	}
	if !Verify([]JournalEvent{first, second}) {
		t.Fatal("expected valid chain")
	}
	second.Event.Path = "/tampered"
	if Verify([]JournalEvent{first, second}) {
		t.Fatal("tampered event must fail verification")
	}
}
