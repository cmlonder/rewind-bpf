// Package branch provides an explicit, conflict-checked Git branch adapter.
// It is intentionally separate from the OverlayFS transaction: applying a
// reviewed candidate to a repository is an operator action, not an automatic
// consequence of an agent run.
package branch

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/acceptance"
	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/export"
	"github.com/rewindbpf/rewind/internal/manifest"
)

type Report struct {
	RepoRoot string        `json:"repo_root"`
	Branch   string        `json:"branch"`
	Changes  []diff.Change `json:"changes"`
	Applied  bool          `json:"applied"`
	CommitID string        `json:"commit_id,omitempty"`
}

// Apply validates a clean checked-out branch, checks destination drift against
// base, validates Git's patch application, and only then mutates the branch.
// When commit is true, the applied changes are committed with message.
func Apply(base, destination, candidate manifest.Manifest, candidateRoot, repoRoot, branchName, message string, commit bool) (Report, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Report{}, fmt.Errorf("resolve Git repository: %w", err)
	}
	resolved, err := git(repoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return Report{}, fmt.Errorf("resolve Git repository: %w", err)
	}
	resolved = filepath.Clean(strings.TrimSpace(resolved))
	canonicalRepo, canonicalErr := filepath.EvalSymlinks(repoRoot)
	canonicalResolved, resolvedErr := filepath.EvalSymlinks(resolved)
	if canonicalErr != nil || resolvedErr != nil || canonicalRepo != canonicalResolved {
		return Report{}, fmt.Errorf("repository path is not the Git root: %s", repoRoot)
	}
	current, err := git(repoRoot, "branch", "--show-current")
	if err != nil {
		return Report{}, fmt.Errorf("read current Git branch: %w", err)
	}
	current = strings.TrimSpace(current)
	if current == "" || current != branchName {
		return Report{}, fmt.Errorf("refusing branch %q: checkout is %q", branchName, current)
	}
	status, err := git(repoRoot, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return Report{}, fmt.Errorf("read Git status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return Report{}, fmt.Errorf("refusing dirty Git worktree")
	}
	report := acceptance.Check(base, destination, candidate)
	if !report.CanApply {
		return Report{RepoRoot: repoRoot, Branch: branchName, Changes: report.Changes}, fmt.Errorf("candidate conflicts with destination: %s", strings.Join(report.Conflicts, ", "))
	}
	paths := make([]string, 0, len(report.Changes))
	for _, change := range report.Changes {
		if change.Path == ".git" || strings.HasPrefix(change.Path, ".git/") {
			return Report{}, fmt.Errorf("refusing Git metadata change %q", change.Path)
		}
		paths = append(paths, change.Path)
	}
	patch, err := export.GitPatchPaths(repoRoot, candidateRoot, paths)
	if err != nil {
		return Report{}, fmt.Errorf("build Git candidate patch: %w", err)
	}
	// patch is intentionally generated only for manifest changes; the check
	// below keeps Git from applying an absolute path if a platform-specific Git
	// prefix differs from the path resolver.
	result := Report{RepoRoot: repoRoot, Branch: branchName, Changes: report.Changes}
	if !export.HasChanges(patch) {
		return result, nil
	}
	if err := gitPatch(repoRoot, patch, true); err != nil {
		return Report{}, fmt.Errorf("Git patch preflight failed: %w", err)
	}
	if err := gitPatch(repoRoot, patch, false); err != nil {
		return Report{}, fmt.Errorf("apply Git patch: %w", err)
	}
	result.Applied = true
	if commit {
		if strings.TrimSpace(message) == "" {
			return result, fmt.Errorf("commit message is required when --commit is enabled")
		}
		if _, err := git(repoRoot, "add", "--all"); err != nil {
			return result, fmt.Errorf("stage Git branch changes: %w", err)
		}
		if _, err := git(repoRoot, "commit", "-m", message); err != nil {
			return result, fmt.Errorf("commit Git branch changes: %w", err)
		}
		commitID, err := git(repoRoot, "rev-parse", "HEAD")
		if err != nil {
			return result, fmt.Errorf("read Git commit: %w", err)
		}
		result.CommitID = strings.TrimSpace(commitID)
	}
	return result, nil
}

func git(repo string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func gitPatch(repo, patch string, checkOnly bool) error {
	args := []string{"-C", repo, "apply", "--binary"}
	if checkOnly {
		args = append(args, "--check")
	}
	command := exec.Command("git", args...)
	command.Stdin = bytes.NewReader([]byte(patch))
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git apply: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
