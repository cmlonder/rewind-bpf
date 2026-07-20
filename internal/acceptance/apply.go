package acceptance

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

// Apply performs an explicit, conflict-checked candidate apply. It is never
// called implicitly by a run: callers must ask for a report and require their
// own confirmation before invoking it.
func Apply(base, destination, candidate manifest.Manifest, candidateRoot, destinationRoot string) (Report, error) {
	report := Check(base, destination, candidate)
	if !report.CanApply {
		return report, fmt.Errorf("candidate conflicts with destination: %s", strings.Join(report.Conflicts, ", "))
	}
	if err := validateRoots(candidateRoot, destinationRoot); err != nil {
		return report, err
	}
	entries := make(map[string]manifest.Entry, len(candidate.Entries))
	for _, entry := range candidate.Entries {
		entries[entry.Path] = entry
	}
	changes := append([]diff.Change(nil), report.Changes...)
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Kind == diff.Deleted && changes[j].Kind != diff.Deleted {
			return false
		}
		if changes[i].Kind != diff.Deleted && changes[j].Kind == diff.Deleted {
			return true
		}
		return changes[i].Path < changes[j].Path
	})
	for _, change := range changes {
		path, err := safeJoin(destinationRoot, change.Path)
		if err != nil {
			return report, err
		}
		switch change.Kind {
		case diff.Deleted:
			if err := os.RemoveAll(path); err != nil {
				return report, fmt.Errorf("remove %s: %w", change.Path, err)
			}
		case diff.Created, diff.Modified:
			entry := entries[change.Path]
			if err := copyEntry(candidateRoot, path, entry); err != nil {
				return report, err
			}
		}
	}
	return report, nil
}

func validateRoots(candidate, destination string) error {
	for label, root := range map[string]string{"candidate": candidate, "destination": destination} {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%s root must be an existing directory: %s", label, root)
		}
	}
	return nil
}

func safeJoin(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe acceptance path %q", relative)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe acceptance path %q", relative)
	}
	joined := filepath.Join(root, clean)
	if rel, err := filepath.Rel(root, joined); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("acceptance path escapes destination: %q", relative)
	}
	return joined, nil
}

func copyEntry(candidateRoot, destination string, entry manifest.Entry) error {
	source, err := safeJoin(candidateRoot, entry.Path)
	if err != nil {
		return err
	}
	if entry.Type == "symlink" || entry.Type == "other" {
		return fmt.Errorf("refusing unsupported candidate entry %q of type %s", entry.Path, entry.Type)
	}
	if entry.Type == "directory" {
		if err := os.MkdirAll(destination, os.FileMode(entry.Mode)); err != nil {
			return fmt.Errorf("create directory %s: %w", entry.Path, err)
		}
		return nil
	}
	if entry.Type != "file" {
		return fmt.Errorf("refusing unknown candidate entry %q", entry.Path)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", entry.Path, err)
	}
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open candidate %s: %w", entry.Path, err)
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".rewind-accept-*")
	if err != nil {
		return fmt.Errorf("create temporary candidate %s: %w", entry.Path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(os.FileMode(entry.Mode)); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set mode %s: %w", entry.Path, err)
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy candidate %s: %w", entry.Path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close candidate %s: %w", entry.Path, err)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return fmt.Errorf("replace candidate %s: %w", entry.Path, err)
	}
	return nil
}
