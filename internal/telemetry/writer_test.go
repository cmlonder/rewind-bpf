package telemetry

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/event"
)

func testEvent() event.Event {
	return event.Event{RunID: "run_test", PID: 42, Operation: event.OpenAt, Path: "/workspace/main.go", TimestampNS: 1, Decision: event.Allow, Risk: event.Medium}
}

func TestJournalWriterCapsWithoutBreakingTheReader(t *testing.T) {
	var output bytes.Buffer
	writer := &JournalWriter{Destination: &output, MaxBytes: 1}
	if err := writer.Append(testEvent()); err != nil {
		t.Fatal(err)
	}
	if !writer.Truncated || output.Len() != 0 {
		t.Fatalf("writer = truncated=%v bytes=%d, want truncated and no partial line", writer.Truncated, output.Len())
	}
	if err := writer.Append(testEvent()); err != nil {
		t.Fatal(err)
	}
	if !writer.Truncated || output.Len() != 0 {
		t.Fatalf("writer changed after cap: truncated=%v bytes=%d", writer.Truncated, output.Len())
	}
}

func TestJournalWriterProducesVerifiableChain(t *testing.T) {
	var output bytes.Buffer
	writer := &JournalWriter{Destination: &output}
	first := testEvent()
	second := testEvent()
	second.TimestampNS = 2
	if err := writer.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(second); err != nil {
		t.Fatal(err)
	}
	if writer.Truncated || writer.Bytes != uint64(output.Len()) || !strings.HasSuffix(output.String(), "\n") {
		t.Fatalf("unexpected writer state: %+v output=%q", writer, output.String())
	}
}
