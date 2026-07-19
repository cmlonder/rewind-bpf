// Package runplan composes the safe, pre-execution inputs for one protected
// agent run. It does not mount filesystems, start processes, or attach eBPF.
package runplan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/capabilities"
	"github.com/rewindbpf/rewind/internal/landlock"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policycompile"
)

type Config struct {
	Workspace      string
	RuntimeRoot    string
	Policy         policy.Policy
	RuntimeRoots   []string
	OverlayBackend overlay.Backend
}

type Plan struct {
	Run            lifecycle.Run           `json:"run"`
	Layout         overlay.Layout          `json:"layout"`
	Manifest       manifest.Manifest       `json:"manifest"`
	ReadRules      policycompile.ReadRules `json:"read_rules"`
	Landlock       *landlock.Plan          `json:"landlock,omitempty"`
	OverlayBackend overlay.Backend         `json:"overlay_backend"`
	CgroupPath     string                  `json:"cgroup_path,omitempty"`
	Capabilities   capabilities.Report     `json:"capabilities,omitempty"`
}

// Build validates and composes all pre-execution state. The workspace is used
// directly as OverlayFS lowerdir, avoiding an upfront copy. The returned plan
// is inert until a future coordinator explicitly mounts, launches, and owns
// cleanup for the run.
func Build(config Config) (Plan, error) {
	workspace, err := resolveWorkspace(config.Workspace)
	if err != nil {
		return Plan{}, err
	}
	if err := config.Policy.Validate(); err != nil {
		return Plan{}, fmt.Errorf("build run plan: policy: %w", err)
	}
	runtimeRoot, err := resolveRuntimeRoot(config.RuntimeRoot)
	if err != nil {
		return Plan{}, err
	}
	if isWithin(workspace, runtimeRoot) || isWithin(runtimeRoot, workspace) {
		return Plan{}, fmt.Errorf("build run plan: runtime root and workspace must not contain one another")
	}

	layout, err := overlay.NewLayoutWithLower(runtimeRoot, workspace)
	if err != nil {
		return Plan{}, fmt.Errorf("build run plan: overlay layout: %w", err)
	}
	snapshot, err := manifest.Build(workspace)
	if err != nil {
		return Plan{}, fmt.Errorf("build run plan: manifest: %w", err)
	}
	readRules, err := policycompile.CompileRead(config.Policy.Read, layout.Merged, snapshot)
	if err != nil {
		return Plan{}, fmt.Errorf("build run plan: read policy: %w", err)
	}

	var landlockPlan *landlock.Plan
	if readRules.Mode == policy.ModeEnforce {
		value, err := landlock.BuildPlan(layout.Merged, readRules, config.RuntimeRoots)
		if err != nil {
			return Plan{}, fmt.Errorf("build run plan: Landlock: %w", err)
		}
		landlockPlan = &value
	}
	run, err := lifecycle.New()
	if err != nil {
		return Plan{}, fmt.Errorf("build run plan: lifecycle: %w", err)
	}
	backend := config.OverlayBackend
	if backend == "" {
		backend = overlay.BackendFuse
	}
	if backend != overlay.BackendFuse && backend != overlay.BackendKernel {
		return Plan{}, fmt.Errorf("build run plan: unsupported overlay backend %q", backend)
	}
	return Plan{Run: run, Layout: layout, Manifest: snapshot, ReadRules: readRules, Landlock: landlockPlan, OverlayBackend: backend}, nil
}

func resolveWorkspace(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("build run plan: workspace cannot be empty")
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("build run plan: resolve workspace: %w", err)
	}
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return "", fmt.Errorf("build run plan: refusing filesystem root as workspace")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("build run plan: stat workspace: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("build run plan: workspace is not a directory: %s", path)
	}
	return path, nil
}

func resolveRuntimeRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("build run plan: runtime root cannot be empty")
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("build run plan: resolve runtime root: %w", err)
	}
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return "", fmt.Errorf("build run plan: refusing filesystem root as runtime root")
	}
	return path, nil
}

func isWithin(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
