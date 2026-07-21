package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

const NativeRecordVersion = 1

type NativeEvent struct {
	Operation string `json:"operation"`
	Path      string `json:"path,omitempty"`
	Decision  string `json:"decision"`
	Timestamp string `json:"timestamp"`
	ExitCode  int    `json:"exit_code,omitempty"`
}

type NativeRecord struct {
	Version      int               `json:"version"`
	RunID        string            `json:"run_id"`
	Platform     string            `json:"platform"`
	Backend      string            `json:"backend"`
	Workspace    string            `json:"workspace"`
	RuntimeRoot  string            `json:"runtime_root"`
	HistoryPath  string            `json:"history_path,omitempty"`
	View         string            `json:"view"`
	PolicyPath   string            `json:"policy_path,omitempty"`
	Command      []string          `json:"command"`
	State        string            `json:"state"`
	ExitCode     int               `json:"exit_code,omitempty"`
	BaseManifest manifest.Manifest `json:"base_manifest"`
	Changes      []diff.Change     `json:"changes,omitempty"`
	EventsPath   string            `json:"events_path,omitempty"`
	Events       []NativeEvent     `json:"events,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func ReadNativeRecord(path string) (NativeRecord, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return NativeRecord{}, fmt.Errorf("read native run record: %w", err)
	}
	var record NativeRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return NativeRecord{}, fmt.Errorf("decode native run record: %w", err)
	}
	if record.Version != NativeRecordVersion {
		return NativeRecord{}, fmt.Errorf("unsupported native run record version %d", record.Version)
	}
	return record, nil
}

func WriteNativeRecord(path string, record NativeRecord) error {
	record.Version = NativeRecordVersion
	record.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode native run record: %w", err)
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create native record directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rewind-native-record-*")
	if err != nil {
		return fmt.Errorf("create native record temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write native run record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close native run record: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish native run record: %w", err)
	}
	return nil
}
