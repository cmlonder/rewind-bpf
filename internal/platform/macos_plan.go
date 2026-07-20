package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MacOSPlan is a read-only prerequisite report for the future native adapter.
// It intentionally does not clone, mount, launch, or delete anything.
type MacOSPlan struct {
	Workspace   string   `json:"workspace"`
	Filesystem  string   `json:"filesystem,omitempty"`
	SandboxExec string   `json:"sandbox_exec,omitempty"`
	Diskutil    string   `json:"diskutil,omitempty"`
	Ready       bool     `json:"ready"`
	Reasons     []string `json:"reasons,omitempty"`
}

// PlanForWorkspace probes only the requested workspace and local tool paths.
// A ready plan still requires the manual disposable-volume gate before it can
// become an enforcing backend.
func PlanForWorkspace(ctx context.Context, workspace string) (MacOSPlan, error) {
	if runtime.GOOS != "darwin" {
		return MacOSPlan{}, fmt.Errorf("macOS native plan requested on %s", runtime.GOOS)
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return MacOSPlan{}, fmt.Errorf("resolve macOS workspace: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return MacOSPlan{}, fmt.Errorf("stat macOS workspace: %w", err)
	}
	if !info.IsDir() {
		return MacOSPlan{}, fmt.Errorf("macOS workspace is not a directory")
	}
	plan := MacOSPlan{Workspace: abs, Reasons: []string{"EndpointSecurity entitlement and APFS disposable-volume rollback are not manually validated"}}
	if path, err := exec.LookPath("sandbox-exec"); err == nil {
		plan.SandboxExec = path
	} else {
		plan.Reasons = append(plan.Reasons, "sandbox-exec is unavailable")
	}
	if path, err := exec.LookPath("diskutil"); err == nil {
		plan.Diskutil = path
	} else {
		plan.Reasons = append(plan.Reasons, "diskutil is unavailable")
	}
	filesystem, err := commandOutput(ctx, "/usr/bin/stat", "-f", "%T", abs)
	if err != nil {
		plan.Reasons = append(plan.Reasons, fmt.Sprintf("filesystem probe failed: %v", err))
	} else {
		plan.Filesystem = strings.TrimSpace(filesystem)
		if plan.Filesystem != "apfs" {
			plan.Reasons = append(plan.Reasons, fmt.Sprintf("workspace filesystem is %q, APFS is required", plan.Filesystem))
		}
	}
	plan.Ready = plan.SandboxExec != "" && plan.Diskutil != "" && plan.Filesystem == "apfs" && len(plan.Reasons) == 1
	return plan, nil
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
