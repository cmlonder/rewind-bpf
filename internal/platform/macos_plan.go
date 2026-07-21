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
	Workspace        string   `json:"workspace"`
	Filesystem       string   `json:"filesystem,omitempty"`
	SandboxExec      string   `json:"sandbox_exec,omitempty"`
	Diskutil         string   `json:"diskutil,omitempty"`
	Ready            bool     `json:"ready"`
	EnforcementReady bool     `json:"enforcement_ready"`
	ManualGate       bool     `json:"manual_gate_required"`
	Reasons          []string `json:"reasons,omitempty"`
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
	plan := MacOSPlan{Workspace: abs, ManualGate: true, Reasons: []string{"EndpointSecurity entitlement and APFS disposable-volume rollback are not manually validated"}}
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
	if plan.Diskutil != "" {
		mountPoint, mountErr := mountPointForPath(ctx, abs)
		if mountErr != nil {
			plan.Reasons = append(plan.Reasons, fmt.Sprintf("mount-point probe failed: %v", mountErr))
		} else {
			filesystemInfo, probeErr := commandOutput(ctx, plan.Diskutil, "info", mountPoint)
			if probeErr != nil {
				plan.Reasons = append(plan.Reasons, fmt.Sprintf("diskutil filesystem probe failed: %v", probeErr))
			} else {
				plan.Filesystem = parseDiskutilFilesystem(filesystemInfo)
			}
		}
	}
	if plan.Filesystem == "" {
		// Keep stat as a read-only fallback for older/macOS-minimal images. BSD
		// stat's %T is not consistent across releases, so an ambiguous value is
		// treated as unknown rather than incorrectly advertised as APFS.
		filesystem, statErr := commandOutput(ctx, "/usr/bin/stat", "-f", "%T", abs)
		if statErr == nil {
			candidate := strings.TrimSpace(filesystem)
			if candidate == "apfs" || candidate == "hfs" || candidate == "ufs" {
				plan.Filesystem = candidate
			}
		} else if plan.Diskutil == "" {
			plan.Reasons = append(plan.Reasons, fmt.Sprintf("filesystem probe failed: %v", statErr))
		}
	}
	if plan.Filesystem != "apfs" {
		plan.Reasons = append(plan.Reasons, fmt.Sprintf("workspace filesystem is %q, APFS is required", plan.Filesystem))
	}
	plan.Ready = plan.SandboxExec != "" && plan.Diskutil != "" && plan.Filesystem == "apfs" && len(plan.Reasons) == 1
	return plan, nil
}

func mountPointForPath(ctx context.Context, path string) (string, error) {
	output, err := commandOutput(ctx, "/bin/df", "-P", path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("unexpected df output")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 6 {
		return "", fmt.Errorf("unexpected df record")
	}
	return fields[len(fields)-1], nil
}

func parseDiskutilFilesystem(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"Type (Bundle):", "File System Personality:"} {
			if strings.HasPrefix(line, prefix) {
				return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
			}
		}
	}
	return ""
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
