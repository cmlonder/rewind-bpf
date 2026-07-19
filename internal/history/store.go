// Package history stores small, durable run summaries. Large event journals
// remain in their run directories; this index is intentionally bounded and
// portable so a future daemon can retain metadata without copying workspaces.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Entry struct {
	RunID      string    `json:"run_id"`
	State      string    `json:"state"`
	Workspace  string    `json:"workspace"`
	RecordPath string    `json:"record_path"`
	UpperBytes int64     `json:"upper_bytes"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Store struct{ path string }

func Open(path string) Store { return Store{path: path} }

func (s Store) List() ([]Entry, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode history: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].UpdatedAt.After(entries[j].UpdatedAt) })
	return entries, nil
}

func (s Store) Upsert(entry Entry) error {
	if entry.RunID == "" {
		return fmt.Errorf("history entry run_id is required")
	}
	entries, err := s.List()
	if err != nil {
		return err
	}
	found := false
	for i := range entries {
		if entries[i].RunID == entry.RunID {
			entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, entry)
	}
	return s.write(entries)
}

func (s Store) PruneKeepLatest(keep int) (int, error) {
	if keep < 0 {
		return 0, fmt.Errorf("history keep count cannot be negative")
	}
	entries, err := s.List()
	if err != nil {
		return 0, err
	}
	if len(entries) <= keep {
		return 0, nil
	}
	removed := len(entries) - keep
	return removed, s.write(entries[:keep])
}

func (s Store) write(entries []Entry) error {
	if s.path == "" {
		return fmt.Errorf("history path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create history directory: %w", err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit history: %w", err)
	}
	return nil
}
