package telemetry

import (
	"encoding/binary"
	"testing"

	"github.com/rewindbpf/rewind/internal/event"
)

func TestDecodeEvent(t *testing.T) {
	raw := make([]byte, WireEventSize)
	binary.LittleEndian.PutUint32(raw[0:4], 1842)
	binary.LittleEndian.PutUint32(raw[4:8], event.OperationCodeUnlinkAt)
	binary.LittleEndian.PutUint64(raw[8:16], 123456789)
	binary.LittleEndian.PutUint32(raw[16:20], event.DecisionCodeAllow)
	binary.LittleEndian.PutUint32(raw[20:24], event.RiskCodeHigh)
	copy(raw[pathOffset:], "/workspace/src/main.go")

	value, err := Decode(raw, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	if value.RunID != "run_test" || value.PID != 1842 || value.Operation != event.UnlinkAt || value.Path != "/workspace/src/main.go" {
		t.Fatalf("decoded event = %+v", value)
	}
	if value.TimestampNS != 123456789 || value.Decision != event.Allow || value.Risk != event.High {
		t.Fatalf("decoded metadata = %+v", value)
	}
}

func TestDecodeRejectsMalformedSamples(t *testing.T) {
	if _, err := Decode(make([]byte, WireEventSize-1), "run_test"); err == nil {
		t.Fatal("expected short sample error")
	}

	raw := make([]byte, WireEventSize)
	binary.LittleEndian.PutUint32(raw[0:4], 42)
	binary.LittleEndian.PutUint32(raw[4:8], 999)
	if _, err := Decode(raw, "run_test"); err == nil {
		t.Fatal("expected unsupported operation error")
	}
	if _, err := Decode(raw, ""); err == nil {
		t.Fatal("expected missing run id error")
	}
}
