// Package landlock owns the userspace plan for Landlock filesystem rules.
// Syscall application is kept in a Linux-only file; this plan remains safe to
// test on the development host.
package landlock

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policycompile"
)

type Plan struct {
	Root         string   `json:"root"`
	AllowedFiles []string `json:"allowed_files"`
	AllowedDirs  []string `json:"allowed_dirs"`
	RuntimeRoots []string `json:"runtime_roots"`
}

// BuildPlan turns compiled enforce-mode rules into the allowlist consumed by
// Landlock. Landlock is an allowlist LSM, so audit mode intentionally produces
// no plan and relies on eBPF telemetry instead.
func BuildPlan(root string, rules policycompile.ReadRules, runtimeRoots []string) (Plan, error) {
	if rules.Mode != policy.ModeEnforce {
		return Plan{}, fmt.Errorf("build Landlock plan: mode must be enforce, got %q", rules.Mode)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, fmt.Errorf("build Landlock plan: resolve root: %w", err)
	}
	plan := Plan{Root: filepath.Clean(root)}
	for _, path := range append(append([]string{}, rules.AllowedFiles...), rules.AllowedDirs...) {
		candidate := filepath.Clean(path)
		if !isWithin(plan.Root, candidate) {
			return Plan{}, fmt.Errorf("build Landlock plan: allowed path escapes root: %s", path)
		}
	}
	for _, root := range runtimeRoots {
		if !filepath.IsAbs(root) {
			return Plan{}, fmt.Errorf("build Landlock plan: runtime root must be absolute: %s", root)
		}
		info, err := os.Stat(root)
		if err != nil {
			return Plan{}, fmt.Errorf("build Landlock plan: runtime root does not exist: %s: %w", root, err)
		}
		if !info.IsDir() {
			return Plan{}, fmt.Errorf("build Landlock plan: runtime root is not a directory: %s", root)
		}
		cleanRoot := filepath.Clean(root)
		if isWithin(plan.Root, cleanRoot) || isWithin(cleanRoot, plan.Root) {
			return Plan{}, fmt.Errorf("build Landlock plan: runtime root overlaps protected workspace: %s", root)
		}
	}
	plan.AllowedFiles = append([]string(nil), rules.AllowedFiles...)
	plan.AllowedDirs = append([]string(nil), rules.AllowedDirs...)
	plan.RuntimeRoots = append([]string(nil), runtimeRoots...)
	sort.Strings(plan.AllowedFiles)
	sort.Strings(plan.AllowedDirs)
	sort.Strings(plan.RuntimeRoots)
	return plan, nil
}

func isWithin(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
