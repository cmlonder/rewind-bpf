package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is a portable description of one path below a manifest root.
// Ownership, xattrs, and timestamps are intentionally deferred to the Linux
// integration stage; Stage 1 focuses on content and structural verification.
type Entry struct {
	Path          string `json:"path"`
	Type          string `json:"type"`
	Mode          uint32 `json:"mode"`
	Size          int64  `json:"size,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
}

type Manifest struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

func Build(root string) (Manifest, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve manifest root: %w", err)
	}
	if info, err := os.Stat(root); err != nil {
		return Manifest{}, fmt.Errorf("stat manifest root: %w", err)
	} else if !info.IsDir() {
		return Manifest{}, fmt.Errorf("manifest root is not a directory: %s", root)
	}

	manifest := Manifest{Version: 1}
	err = filepath.WalkDir(root, func(path string, dirEntry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("lstat %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path %s: %w", path, err)
		}
		entry := Entry{
			Path: filepath.ToSlash(rel),
			Mode: uint32(info.Mode().Perm()),
		}

		switch {
		case info.IsDir():
			entry.Type = "directory"
		case info.Mode().IsRegular():
			entry.Type = "file"
			entry.Size = info.Size()
			entry.SHA256, err = hashFile(path)
			if err != nil {
				return err
			}
		case info.Mode()&os.ModeSymlink != 0:
			entry.Type = "symlink"
			entry.SymlinkTarget, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read symlink %s: %w", path, err)
			}
		default:
			entry.Type = "other"
		}

		manifest.Entries = append(manifest.Entries, entry)
		return nil
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("walk manifest root: %w", err)
	}

	sort.Slice(manifest.Entries, func(i, j int) bool {
		return manifest.Entries[i].Path < manifest.Entries[j].Path
	})
	return manifest, nil
}

func Verify(root string, expected Manifest) error {
	actual, err := Build(root)
	if err != nil {
		return err
	}
	if actual.Version != expected.Version {
		return fmt.Errorf("manifest version mismatch: expected %d, got %d", expected.Version, actual.Version)
	}
	if len(actual.Entries) != len(expected.Entries) {
		return fmt.Errorf("entry count mismatch: expected %d, got %d", len(expected.Entries), len(actual.Entries))
	}
	for i := range expected.Entries {
		if actual.Entries[i] != expected.Entries[i] {
			return fmt.Errorf("entry mismatch at %s: expected %+v, got %+v", expected.Entries[i].Path, expected.Entries[i], actual.Entries[i])
		}
	}
	return nil
}

func WriteJSON(w io.Writer, value Manifest) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func ReadJSON(r io.Reader) (Manifest, error) {
	var value Manifest
	if err := json.NewDecoder(r).Decode(&value); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return value, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s for hashing: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// NormalizePath is used by policy and fixture code to compare slash-separated
// paths consistently across development hosts.
func NormalizePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "." {
		return ""
	}
	return strings.TrimPrefix(path, "./")
}
