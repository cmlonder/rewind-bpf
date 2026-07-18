// Package runstore persists the small amount of metadata needed to inspect or
// roll back a completed protected run from a later CLI invocation.
package runstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rewindbpf/rewind/internal/runplan"
)

type Record struct {
	Plan       runplan.Plan `json:"plan"`
	EventsPath string       `json:"events_path,omitempty"`
}

func Write(path string, record Record) error {
	if path == "" {
		return fmt.Errorf("write run record: path cannot be empty")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("write run record: resolve path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write run record: create parent: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rewind-record-*")
	if err != nil {
		return fmt.Errorf("write run record: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write run record: chmod temporary file: %w", err)
	}
	encoderErr := json.NewEncoder(tmp).Encode(record)
	closeErr := tmp.Close()
	if encoderErr != nil {
		return fmt.Errorf("write run record: encode: %w", encoderErr)
	}
	if closeErr != nil {
		return fmt.Errorf("write run record: close: %w", closeErr)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write run record: replace %s: %w", path, err)
	}
	return nil
}

func Read(path string) (Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return Record{}, fmt.Errorf("read run record %s: %w", path, err)
	}
	defer file.Close()
	var record Record
	if err := json.NewDecoder(file).Decode(&record); err != nil {
		return Record{}, fmt.Errorf("read run record %s: decode: %w", path, err)
	}
	if record.Plan.Run.ID == "" {
		return Record{}, fmt.Errorf("read run record %s: missing run id", path)
	}
	return record, nil
}
