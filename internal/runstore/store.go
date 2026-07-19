// Package runstore persists the small amount of metadata needed to inspect or
// roll back a completed protected run from a later CLI invocation.
package runstore

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rewindbpf/rewind/internal/runplan"
)

type Record struct {
	Plan        runplan.Plan  `json:"plan"`
	EventsPath  string        `json:"events_path,omitempty"`
	EventsPaths []string      `json:"events_paths,omitempty"`
	Events      EventEvidence `json:"events,omitempty"`
}

type EventEvidence struct {
	Count     uint64 `json:"count"`
	Bytes     uint64 `json:"bytes"`
	Dropped   uint64 `json:"dropped,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Complete  bool   `json:"complete"`
}

// SummarizeEvents computes a portable evidence digest without parsing or
// exposing event paths. Count is newline-delimited record count; malformed
// lines make Complete false so callers cannot claim a complete audit stream.
func SummarizeEvents(path string) (EventEvidence, error) {
	return SummarizeEventsPaths([]string{path})
}

// SummarizeEventsPaths computes one ordered digest over a rotated JSONL
// stream. The first path is the original events.jsonl file; subsequent paths
// are appended in the order supplied by the writer.
func SummarizeEventsPaths(paths []string) (EventEvidence, error) {
	if len(paths) == 0 || (len(paths) == 1 && paths[0] == "") {
		return EventEvidence{Complete: true}, nil
	}
	hash := sha256.New()
	count := uint64(0)
	complete := true
	var totalBytes uint64
	for _, path := range paths {
		if path == "" {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return EventEvidence{}, fmt.Errorf("summarize events: open: %w", err)
		}
		reader := bufio.NewReader(file)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				_, _ = hash.Write(line)
				totalBytes += uint64(len(line))
				if line[len(line)-1] == '\n' {
					count++
					if !json.Valid(bytes.TrimSpace(line)) {
						complete = false
					}
				} else {
					complete = false
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					_ = file.Close()
					return EventEvidence{}, fmt.Errorf("summarize events: read: %w", readErr)
				}
				break
			}
		}
		if err := file.Close(); err != nil {
			return EventEvidence{}, fmt.Errorf("summarize events: close: %w", err)
		}
	}
	return EventEvidence{Count: count, Bytes: totalBytes, SHA256: fmt.Sprintf("%x", hash.Sum(nil)), Complete: complete}, nil
}

// EventLogPaths returns the recorded rotation order while preserving
// compatibility with records written before rotation support existed.
func EventLogPaths(record Record) []string {
	if len(record.EventsPaths) > 0 {
		return append([]string(nil), record.EventsPaths...)
	}
	if record.EventsPath == "" {
		return nil
	}
	return []string{record.EventsPath}
}

// WithDropped marks the evidence incomplete when the kernel ring buffer had
// to discard records. Keeping this transformation in runstore makes the
// persisted completeness rule independent of the sensor implementation.
func (e EventEvidence) WithDropped(dropped uint64) EventEvidence {
	e.Dropped = dropped
	if dropped > 0 {
		e.Complete = false
	}
	return e
}

// WithTruncated records a userspace byte cap or rotation boundary. A capped
// stream remains useful for diagnostics but must never be reported complete.
func (e EventEvidence) WithTruncated(truncated bool) EventEvidence {
	e.Truncated = truncated
	if truncated {
		e.Complete = false
	}
	return e
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
	if err := chownToInvoker(path); err != nil {
		return fmt.Errorf("write run record: restore invoker ownership: %w", err)
	}
	return nil
}

// chownToInvoker keeps records readable after a privileged `sudo rewind run`.
// It is a no-op for unprivileged invocations and when no SUDO identity exists.
func chownToInvoker(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	uid, err := strconv.Atoi(os.Getenv("SUDO_UID"))
	if err != nil || uid < 1 {
		return nil
	}
	gid, err := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err != nil || gid < 1 {
		return nil
	}
	return os.Chown(path, uid, gid)
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
