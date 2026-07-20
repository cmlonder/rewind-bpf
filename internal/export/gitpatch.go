package export

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// GitPatch renders a full-fidelity, review-only diff using git's --no-index
// mode. Unlike UnifiedPatch it can represent binary files, directories, and
// mode changes. Git is invoked without a shell and its absolute-root prefixes
// are rewritten to stable a/ and b/ paths before returning the artifact.
func GitPatch(beforeRoot, afterRoot string) (string, error) {
	before, err := filepath.Abs(beforeRoot)
	if err != nil {
		return "", fmt.Errorf("build git patch: resolve lower root: %w", err)
	}
	after, err := filepath.Abs(afterRoot)
	if err != nil {
		return "", fmt.Errorf("build git patch: resolve merged root: %w", err)
	}
	command := exec.Command("git", "diff", "--no-index", "--binary", "--src-prefix=a/", "--dst-prefix=b/", "--", before, after)
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			return "", fmt.Errorf("build git patch: run git diff: %w", err)
		}
	}
	return rewriteGitRoots(string(output), filepath.ToSlash(before), filepath.ToSlash(after)), nil
}

func rewriteGitRoots(patch, before, after string) string {
	before = strings.TrimPrefix(before, "/")
	after = strings.TrimPrefix(after, "/")
	patch = strings.ReplaceAll(patch, "a/"+before+"/", "a/")
	patch = strings.ReplaceAll(patch, "b/"+before+"/", "a/")
	patch = strings.ReplaceAll(patch, "a/"+after+"/", "b/")
	patch = strings.ReplaceAll(patch, "b/"+after+"/", "b/")
	lines := strings.Split(patch, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}
		if strings.HasPrefix(fields[2], "b/") && strings.HasPrefix(fields[3], "b/") {
			fields[2] = "a/" + strings.TrimPrefix(fields[2], "b/")
		}
		if strings.HasPrefix(fields[2], "a/") && strings.HasPrefix(fields[3], "a/") {
			fields[3] = "b/" + strings.TrimPrefix(fields[3], "a/")
		}
		lines[i] = strings.Join(fields, " ")
	}
	return strings.Join(lines, "\n")
}

// HasChanges reports whether a native Git diff contains any output.
func HasChanges(patch string) bool { return len(bytes.TrimSpace([]byte(patch))) > 0 }

// GitPatchPaths renders only the requested relative paths. It is used by the
// branch adapter so a workspace that contains a .git directory never causes
// Git metadata to enter an acceptance patch.
func GitPatchPaths(beforeRoot, afterRoot string, paths []string) (string, error) {
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "." || path == ".." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			return "", fmt.Errorf("unsafe Git patch path %q", path)
		}
		unique[path] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for path := range unique {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var patches strings.Builder
	for _, relative := range ordered {
		beforePath := filepath.Join(beforeRoot, filepath.FromSlash(relative))
		afterPath := filepath.Join(afterRoot, filepath.FromSlash(relative))
		beforeInfo, beforeErr := os.Lstat(beforePath)
		afterInfo, afterErr := os.Lstat(afterPath)
		if beforeErr != nil && !os.IsNotExist(beforeErr) {
			return "", fmt.Errorf("stat Git patch source %s: %w", relative, beforeErr)
		}
		if afterErr != nil && !os.IsNotExist(afterErr) {
			return "", fmt.Errorf("stat Git patch candidate %s: %w", relative, afterErr)
		}
		if beforeErr != nil && afterErr != nil {
			continue
		}
		if (beforeErr == nil && beforeInfo.IsDir()) || (afterErr == nil && afterInfo.IsDir()) {
			continue
		}
		if beforeErr != nil {
			beforePath = os.DevNull
		}
		if afterErr != nil {
			afterPath = os.DevNull
		}
		patch, err := gitDiffPair(beforePath, afterPath)
		if err != nil {
			return "", fmt.Errorf("build Git patch for %s: %w", relative, err)
		}
		patch = rewriteGitRoots(patch, filepath.ToSlash(beforeRoot), filepath.ToSlash(afterRoot))
		if strings.TrimSpace(patch) != "" {
			patches.WriteString(patch)
			if !strings.HasSuffix(patch, "\n") {
				patches.WriteByte('\n')
			}
		}
	}
	return patches.String(), nil
}

func gitDiffPair(before, after string) (string, error) {
	command := exec.Command("git", "diff", "--no-index", "--binary", "--src-prefix=a/", "--dst-prefix=b/", "--", before, after)
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			return "", fmt.Errorf("run git diff: %w", err)
		}
	}
	return string(output), nil
}
