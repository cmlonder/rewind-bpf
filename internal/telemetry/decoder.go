// Package telemetry decodes the compact records emitted by the eBPF sensors.
// It does not load programs, attach hooks, or make policy decisions.
package telemetry

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/rewindbpf/rewind/internal/event"
)

const (
	pathOffset    = 24
	pathLength    = 256
	WireEventSize = pathOffset + pathLength
)

// Decode converts one raw ring-buffer sample into the userspace event model.
// The kernel record intentionally omits the string run ID; userspace supplies
// the active run context when decoding.
func Decode(raw []byte, runID string) (event.Event, error) {
	if runID == "" {
		return event.Event{}, fmt.Errorf("decode eBPF event: run id cannot be empty")
	}
	if len(raw) < WireEventSize {
		return event.Event{}, fmt.Errorf("decode eBPF event: sample has %d bytes, need at least %d", len(raw), WireEventSize)
	}

	operation, ok := event.OperationFromCode(binary.LittleEndian.Uint32(raw[4:8]))
	if !ok {
		return event.Event{}, fmt.Errorf("decode eBPF event: unsupported operation code %d", binary.LittleEndian.Uint32(raw[4:8]))
	}
	decision, ok := event.DecisionFromCode(binary.LittleEndian.Uint32(raw[16:20]))
	if !ok {
		return event.Event{}, fmt.Errorf("decode eBPF event: unsupported decision code %d", binary.LittleEndian.Uint32(raw[16:20]))
	}
	risk, ok := event.RiskFromCode(binary.LittleEndian.Uint32(raw[20:24]))
	if !ok {
		return event.Event{}, fmt.Errorf("decode eBPF event: unsupported risk code %d", binary.LittleEndian.Uint32(raw[20:24]))
	}

	path := raw[pathOffset : pathOffset+pathLength]
	if end := bytes.IndexByte(path, 0); end >= 0 {
		path = path[:end]
	}
	value := event.Event{
		RunID:       runID,
		PID:         binary.LittleEndian.Uint32(raw[0:4]),
		Operation:   operation,
		Path:        string(path),
		TimestampNS: binary.LittleEndian.Uint64(raw[8:16]),
		Decision:    decision,
		Risk:        risk,
	}
	if err := value.Validate(); err != nil {
		return event.Event{}, fmt.Errorf("decode eBPF event: %w", err)
	}
	return value, nil
}
