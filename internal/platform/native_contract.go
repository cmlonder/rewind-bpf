package platform

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// NativeContract describes the platform-specific primitives required before
// enforcement can be enabled. It is portable metadata, not a claim that the
// current host can safely enforce the contract.
type NativeContract struct {
	Platform         string   `json:"platform"`
	Workspace        string   `json:"workspace"`
	ReadBoundary     string   `json:"read_boundary"`
	ProcessBoundary  string   `json:"process_boundary"`
	SnapshotBoundary string   `json:"snapshot_boundary"`
	Ready            bool     `json:"ready"`
	Reasons          []string `json:"reasons,omitempty"`
}

func BuildNativeContract(platformName, workspace string) (NativeContract, error) {
	platformName = strings.ToLower(strings.TrimSpace(platformName))
	if platformName != "darwin" && platformName != "windows" {
		return NativeContract{}, fmt.Errorf("unsupported native platform %q", platformName)
	}
	if strings.TrimSpace(workspace) == "" {
		return NativeContract{}, fmt.Errorf("native workspace is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return NativeContract{}, err
	}
	contract := NativeContract{Platform: platformName, Workspace: abs, Ready: false}
	if platformName == "darwin" {
		contract.ReadBoundary = "Seatbelt profile + EndpointSecurity"
		contract.ProcessBoundary = "EndpointSecurity child-process policy"
		contract.SnapshotBoundary = "APFS clone/disposable volume"
		contract.Reasons = []string{"requires signed EndpointSecurity entitlement", "requires disposable APFS volume acceptance test"}
	} else {
		contract.ReadBoundary = "filesystem minifilter policy"
		contract.ProcessBoundary = "Job Object + restricted token"
		contract.SnapshotBoundary = "VHDX differencing disk"
		contract.Reasons = []string{"requires signed minifilter/service", "requires disposable VHDX acceptance test"}
	}
	return contract, nil
}

func (c NativeContract) SupportedOnHost() bool { return runtime.GOOS == c.Platform }

func SeatbeltProfile(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("Seatbelt workspace is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	quoted := strings.ReplaceAll(abs, "\\", "\\\\")
	quoted = strings.ReplaceAll(quoted, "\"", "\\\"")
	return fmt.Sprintf("(version 1)\n(deny default)\n(allow process*)\n(allow file-read* (subpath \"%s\"))\n(allow file-write* (subpath \"%s\"))\n", quoted, quoted), nil
}

func WindowsJobPlan() []string {
	return []string{"CreateJobObject(REWIND_JOB)", "SetInformationJobObject(JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE)", "AssignProcessToJobObject(REWIND_JOB)", "Create restricted token before CreateProcess"}
}
