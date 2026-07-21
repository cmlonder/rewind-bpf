package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// WindowsPlan is a read-only prerequisite report. It never changes ACLs,
// creates a VHD, or launches a protected process.
type WindowsPlan struct {
	Workspace        string   `json:"workspace"`
	PowerShell       string   `json:"powershell,omitempty"`
	Fsutil           string   `json:"fsutil,omitempty"`
	Ready            bool     `json:"ready"`
	EnforcementReady bool     `json:"enforcement_ready"`
	ManualGate       bool     `json:"manual_gate_required"`
	Reasons          []string `json:"reasons,omitempty"`
}

func PlanForWindowsWorkspace(ctx context.Context, workspace string) (WindowsPlan, error) {
	if runtime.GOOS != "windows" {
		return WindowsPlan{}, fmt.Errorf("Windows native plan requested on %s", runtime.GOOS)
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return WindowsPlan{}, fmt.Errorf("resolve Windows workspace: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return WindowsPlan{}, fmt.Errorf("stat Windows workspace: %w", err)
	}
	if !info.IsDir() {
		return WindowsPlan{}, fmt.Errorf("Windows workspace is not a directory")
	}
	plan := WindowsPlan{Workspace: abs, ManualGate: true, Reasons: []string{"Windows Job Object, filesystem minifilter, and disposable VHDX rollback are not manually validated"}}
	if path, err := exec.LookPath("powershell.exe"); err == nil {
		plan.PowerShell = path
	} else {
		plan.Reasons = append(plan.Reasons, "powershell.exe is unavailable")
	}
	if path, err := exec.LookPath("fsutil.exe"); err == nil {
		plan.Fsutil = path
	} else {
		plan.Reasons = append(plan.Reasons, "fsutil.exe is unavailable")
	}
	plan.Ready = false
	return plan, nil
}
