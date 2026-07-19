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
	Plan       runplan.Plan  `json:"plan"`
	EventsPath string        `json:"events_path,omitempty"`
	Events     EventEvidence `json:"events,omitempty"`
}

type EventEvidence struct {
	Count    uint64 `json:"count"`
	Bytes    uint64 `json:"bytes"`
	SHA256   string `json:"sha256,omitempty"`
	Complete bool   `json:"complete"`
}

// SummarizeEvents computes a portable evidence digest without parsing or
// exposing event paths. Count is newline-delimited record count; malformed
// lines make Complete false so callers cannot claim a complete audit stream.
func SummarizeEvents(path string) (EventEvidence, error) {
	if path == "" {
		return EventEvidence{Complete: true}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EventEvidence{Complete: true}, nil
		}
		return EventEvidence{}, fmt.Errorf("summarize events: open: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	count := uint64(0)
	complete := true
	reader := bufio.NewReader(file)
	var totalBytes uint64
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
				return EventEvidence{}, fmt.Errorf("summarize events: read: %w", readErr)
			}
			break
		}
	}
	return EventEvidence{Count: count, Bytes: totalBytes, SHA256: fmt.Sprintf("%x", hash.Sum(nil)), Complete: complete}, nil
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
