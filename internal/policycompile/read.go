// Package policycompile translates userspace policies into kernel-friendly
// rules. It keeps glob matching and manifest traversal out of the BPF hot path.
package policycompile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/policy"
)

// One byte is reserved for the NUL terminator in the fixed 256-byte BPF key.
const MaxKernelPathBytes = 255

type ReadRule struct {
	Path     string
	Decision string
}

type ReadRules struct {
	Mode         policy.Mode
	Rules        []ReadRule
	AllowedFiles []string
	AllowedDirs  []string
}

// CompileRead expands read globs against the immutable start-of-run manifest.
// The result is deterministic and suitable for loading into a fixed-size BPF
// map. Newly created paths are intentionally outside this first MVP contract.
func CompileRead(read policy.ReadPolicy, root string, snapshot manifest.Manifest) (ReadRules, error) {
	if err := read.Validate(); err != nil {
		return ReadRules{}, fmt.Errorf("compile read policy: %w", err)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return ReadRules{}, fmt.Errorf("compile read policy: resolve root: %w", err)
	}
	root = filepath.Clean(root)
	result := ReadRules{Mode: read.Mode}
	if read.Mode == policy.ModeOff {
		return result, nil
	}

	byPath := make(map[string]string)
	allowedFiles := make([]string, 0)
	allowedDirs := make([]string, 0)
	for _, entry := range snapshot.Entries {
		if entry.Type != "file" && entry.Type != "directory" {
			continue
		}
		candidate := filepath.Join(root, filepath.FromSlash(manifest.NormalizePath(entry.Path)))
		candidate = filepath.Clean(candidate)
		if !isWithin(root, candidate) {
			return ReadRules{}, fmt.Errorf("compile read policy: manifest path escapes root: %s", entry.Path)
		}
		if len([]byte(candidate)) > MaxKernelPathBytes {
			return ReadRules{}, fmt.Errorf("compile read policy: path exceeds %d bytes: %s", MaxKernelPathBytes, candidate)
		}
		decision := read.Decision(candidate)
		if decision == "allow" {
			if read.Mode == policy.ModeEnforce {
				if entry.Type == "file" {
					allowedFiles = append(allowedFiles, candidate)
				} else {
					allowedDirs = append(allowedDirs, candidate)
				}
			}
			continue
		}
		byPath[candidate] = decision
	}

	result.Rules = make([]ReadRule, 0, len(byPath))
	for path, decision := range byPath {
		result.Rules = append(result.Rules, ReadRule{Path: path, Decision: decision})
	}
	sort.Slice(result.Rules, func(i, j int) bool { return result.Rules[i].Path < result.Rules[j].Path })
	sort.Strings(allowedFiles)
	sort.Strings(allowedDirs)
	result.AllowedFiles = allowedFiles
	result.AllowedDirs = allowedDirs
	return result, nil
}

func isWithin(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
