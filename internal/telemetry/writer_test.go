package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestJournalWriterRotatesWithoutResettingChain(t *testing.T) {
	outputs := []*bytes.Buffer{&bytes.Buffer{}}
	writer := &JournalWriter{
		Destination: outputs[0],
		RotateBytes: 1,
		Rotate: func() (io.Writer, error) {
			value := &bytes.Buffer{}
			outputs = append(outputs, value)
			return value, nil
		},
	}
	first := testEvent()
	second := testEvent()
	second.TimestampNS = 2
	if err := writer.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(second); err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 2 || outputs[0].Len() == 0 || outputs[1].Len() == 0 {
		t.Fatalf("rotation outputs = %d, lengths %d/%d", len(outputs), outputs[0].Len(), outputs[1].Len())
	}
	var journal []JournalEvent
	for _, output := range outputs {
		var value JournalEvent
		if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &value); err != nil {
			t.Fatal(err)
		}
		journal = append(journal, value)
	}
	if !Verify(journal) || writer.Bytes != uint64(outputs[0].Len()+outputs[1].Len()) {
		t.Fatalf("rotated chain invalid or byte count wrong: valid=%v bytes=%d", Verify(journal), writer.Bytes)
	}
}
