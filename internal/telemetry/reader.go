package telemetry

import (
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/rewindbpf/rewind/internal/event"
)

// Reader adapts a cilium/ebpf ring-buffer reader to the RewindBPF event model.
// Loading the eBPF collection and obtaining the map remain daemon concerns.
type Reader struct {
	reader *ringbuf.Reader
	runID  string
}

func NewReader(eventsMap *ebpf.Map, runID string) (*Reader, error) {
	if eventsMap == nil {
		return nil, fmt.Errorf("create telemetry reader: nil events map")
	}
	if runID == "" {
		return nil, fmt.Errorf("create telemetry reader: empty run id")
	}
	reader, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		return nil, fmt.Errorf("create telemetry ring-buffer reader: %w", err)
	}
	return &Reader{reader: reader, runID: runID}, nil
}

func (r *Reader) Read() (event.Event, error) {
	if r == nil || r.reader == nil {
		return event.Event{}, fmt.Errorf("read telemetry event: reader is not initialized")
	}
	record, err := r.reader.Read()
	if err != nil {
		return event.Event{}, fmt.Errorf("read telemetry ring-buffer: %w", err)
	}
	return Decode(record.RawSample, r.runID)
}

func (r *Reader) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}
