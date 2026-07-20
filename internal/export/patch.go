package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/diff"
)

// UnifiedPatch renders the regular-text-file portion of a review bundle as a
// conventional unified diff. It is review-only: it reads the lower and merged
// trees and never applies or mutates them. Unsupported entries are refused
// rather than silently omitted from the patch artifact.
func UnifiedPatch(bundle Bundle, beforeRoot, afterRoot string) (string, error) {
	if strings.TrimSpace(beforeRoot) == "" || strings.TrimSpace(afterRoot) == "" {
		return "", fmt.Errorf("build patch: both manifest roots are required")
	}
	var out strings.Builder
	for _, change := range bundle.Changes {
		if (change.Before != nil && change.Before.Type != "file") || (change.After != nil && change.After.Type != "file") {
			return "", fmt.Errorf("build patch: %s change is not a regular file", change.Path)
		}
		if !safePatchPath(change.Path) {
			return "", fmt.Errorf("build patch: unsafe path %q", change.Path)
		}
		var before, after []byte
		var err error
		if change.Kind != diff.Created {
			before, err = readTextFile(beforeRoot, change.Path)
			if err != nil {
				return "", err
			}
		}
		if change.Kind != diff.Deleted {
			after, err = readTextFile(afterRoot, change.Path)
			if err != nil {
				return "", err
			}
		}
		writeFilePatch(&out, change, before, after)
	}
	return out.String(), nil
}

func readTextFile(root, relative string) ([]byte, error) {
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("build patch: read %s: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("build patch: %s is not a regular file", relative)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("build patch: read %s: %w", relative, err)
	}
	if bytes.IndexByte(value, 0) >= 0 {
		return nil, fmt.Errorf("build patch: %s is binary", relative)
	}
	return value, nil
}

func writeFilePatch(out *strings.Builder, change diff.Change, before, after []byte) {
	path := change.Path
	oldLines := patchLines(before)
	newLines := patchLines(after)
	from, to := "a/"+path, "b/"+path
	if change.Kind == diff.Created {
		from = "/dev/null"
	}
	if change.Kind == diff.Deleted {
		to = "/dev/null"
	}
	fmt.Fprintf(out, "diff --git a/%s b/%s\n--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", path, path, from, to, lineStart(len(oldLines)), len(oldLines), lineStart(len(newLines)), len(newLines))
	for _, line := range oldLines {
		out.WriteByte('-')
		out.WriteString(line)
		out.WriteByte('\n')
	}
	for _, line := range newLines {
		out.WriteByte('+')
		out.WriteString(line)
		out.WriteByte('\n')
	}
}

func patchLines(value []byte) []string {
	if len(value) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(value), "\n"), "\n")
}

func lineStart(lines int) int {
	if lines == 0 {
		return 0
	}
	return 1
}

func safePatchPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != "" && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
}

// WritePatch atomically writes a text patch with restrictive permissions.
func WritePatch(path, patch string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("write patch: path cannot be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("write patch: resolve path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("write patch: create parent: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".rewind-patch-*")
	if err != nil {
		return fmt.Errorf("write patch: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write patch: chmod: %w", err)
	}
	if _, err := tmp.WriteString(patch); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write patch: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write patch: close: %w", err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		return fmt.Errorf("write patch: replace: %w", err)
	}
	return nil
}
