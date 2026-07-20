package telemetry

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/rewindbpf/rewind/internal/event"
)

// JournalWriter serializes hash-chained events and optionally caps the bytes
// persisted to the destination. Once the cap is reached it keeps consuming
// events at the caller's pace but intentionally drops them from the durable
// log and marks the stream truncated. This prevents an unbounded audit file
// without pretending that the evidence is complete.
type JournalWriter struct {
	Destination io.Writer
	MaxBytes    uint64
	RotateBytes uint64
	Bytes       uint64
	FileBytes   uint64
	Truncated   bool
	Dropped     uint64
	// OnDrop is optional observability for a bounded stream. A dropped event
	// is never reported as durable evidence.
	OnDrop func(event.Event)
	Rotate func() (io.Writer, error)
	chain  Chain
}

func (w *JournalWriter) Append(value event.Event) error {
	if w == nil || w.Destination == nil {
		return fmt.Errorf("write journal event: destination is required")
	}
	journal, err := w.chain.Append(value)
	if err != nil {
		return err
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return fmt.Errorf("marshal journal event: %w", err)
	}
	data = append(data, '\n')
	if w.MaxBytes > 0 && w.Bytes+uint64(len(data)) > w.MaxBytes {
		w.Truncated = true
		w.Dropped++
		if w.OnDrop != nil {
			w.OnDrop(value)
		}
		return nil
	}
	if w.RotateBytes > 0 && w.FileBytes > 0 && w.FileBytes+uint64(len(data)) > w.RotateBytes {
		if w.Rotate == nil {
			return fmt.Errorf("rotate journal event: destination callback is required")
		}
		destination, err := w.Rotate()
		if err != nil {
			return fmt.Errorf("rotate journal event: %w", err)
		}
		w.Destination = destination
		w.FileBytes = 0
	}
	if _, err := w.Destination.Write(data); err != nil {
		return fmt.Errorf("write journal event: %w", err)
	}
	w.Bytes += uint64(len(data))
	w.FileBytes += uint64(len(data))
	return nil
}
