// Package export creates reviewable, non-mutating change bundles for a live
// protected run. It never writes to the workspace or applies agent changes.
package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

type Bundle struct {
	Version   int               `json:"version"`
	RunID     string            `json:"run_id"`
	CreatedAt time.Time         `json:"created_at"`
	Before    manifest.Manifest `json:"before"`
	After     manifest.Manifest `json:"after"`
	Changes   []diff.Change     `json:"changes"`
}

func Build(runID string, before, after manifest.Manifest) (Bundle, error) {
	if runID == "" {
		return Bundle{}, fmt.Errorf("build export bundle: run id cannot be empty")
	}
	return Bundle{
		Version:   1,
		RunID:     runID,
		CreatedAt: time.Now().UTC(),
		Before:    before,
		After:     after,
		Changes:   diff.Compare(before, after),
	}, nil
}

func Write(path string, bundle Bundle) error {
	if path == "" {
		return fmt.Errorf("write export bundle: path cannot be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("write export bundle: resolve path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("write export bundle: create parent: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".rewind-export-*")
	if err != nil {
		return fmt.Errorf("write export bundle: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write export bundle: chmod: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bundle); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write export bundle: encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write export bundle: close: %w", err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		return fmt.Errorf("write export bundle: replace: %w", err)
	}
	return nil
}
