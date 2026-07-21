// Package platform describes host backends without silently downgrading a
// protected run. The report is safe to call on any OS and performs no setup.
package platform

import (
	"fmt"
	"runtime"
	"strings"
)

type Capability struct {
	Platform             string   `json:"platform"`
	Backend              string   `json:"backend"`
	Supported            bool     `json:"supported"`
	ProjectIsolation     bool     `json:"project_isolation"`
	SensitiveReadDeny    bool     `json:"sensitive_read_deny"`
	NetworkEgressControl bool     `json:"network_egress_control"`
	Reasons              []string `json:"reasons,omitempty"`
}

// PlatformStatus is the operator-facing, read-only matrix used by the CLI and
// the UI.  It deliberately separates code availability from privileged
// acceptance: a target can be cross-compiled and contract-complete while its
// signed helper still awaits a disposable VM/volume test.
type PlatformStatus struct {
	Platform          string   `json:"platform"`
	Host              bool     `json:"host"`
	BuildTarget       string   `json:"build_target"`
	Backend           string   `json:"backend"`
	CodeComplete      bool     `json:"code_complete"`
	EnforcementReady  bool     `json:"enforcement_ready"`
	ManualGate        bool     `json:"manual_gate_required"`
	HelperVerified    bool     `json:"helper_verified"`
	ProjectIsolation  bool     `json:"project_isolation"`
	SensitiveReadDeny bool     `json:"sensitive_read_deny"`
	NetworkControl    bool     `json:"network_egress_control"`
	Reasons           []string `json:"reasons,omitempty"`
}

// StatusMatrix never mutates the host.  The helper manifest is optional; if
// omitted, native targets remain explicitly manual-gated and fail closed.
func StatusMatrix(helperManifest string) ([]PlatformStatus, error) {
	reports := []PlatformStatus{
		{Platform: "linux", BuildTarget: "linux/amd64, linux/arm64", Backend: "overlayfs-landlock-ebpf", CodeComplete: true, EnforcementReady: runtime.GOOS == "linux", ProjectIsolation: true, SensitiveReadDeny: true, Reasons: []string{"privileged OverlayFS/eBPF/cgroup acceptance belongs in the disposable Ubuntu VM"}},
		{Platform: "darwin", BuildTarget: "darwin/arm64", Backend: "Seatbelt + EndpointSecurity + APFS", CodeComplete: true, ManualGate: true, Reasons: []string{"signed EndpointSecurity helper and disposable APFS rollback acceptance are required"}},
		{Platform: "windows", BuildTarget: "windows/amd64", Backend: "Job Object + minifilter + VHDX", CodeComplete: true, ManualGate: true, Reasons: []string{"signed minifilter/service and disposable VHDX rollback acceptance are required"}},
	}
	for i := range reports {
		reports[i].Host = runtime.GOOS == reports[i].Platform
		if strings.TrimSpace(helperManifest) == "" || reports[i].Platform == "linux" {
			continue
		}
		if !reports[i].Host {
			reports[i].Reasons = append(reports[i].Reasons, "helper verification is deferred until the target host runs the preflight")
			continue
		}
		verification, err := VerifyNativeHelper(helperManifest, nil)
		if err != nil {
			return nil, err
		}
		reports[i].HelperVerified = verification.Verified
		if verification.Verified {
			reports[i].Reasons = append(reports[i].Reasons, "helper bytes and optional signature verified; privileged acceptance is still a manual gate")
		} else {
			reports[i].Reasons = append(reports[i].Reasons, verification.Reasons...)
		}
	}
	return reports, nil
}

func Probe() Capability {
	switch runtime.GOOS {
	case "linux":
		return Capability{Platform: "linux", Backend: "overlayfs-landlock-ebpf", Supported: true, ProjectIsolation: true, SensitiveReadDeny: true, Reasons: []string{"requires runtime capability probe for the selected kernel backend"}}
	case "darwin":
		return Capability{Platform: "darwin", Backend: "seatbelt-apfs", Reasons: []string{"read-only prerequisite probe is available", "EndpointSecurity entitlement and APFS disposable-volume rollback require manual validation", "native backend remains fail-closed until that gate passes"}}
	case "windows":
		return Capability{Platform: "windows", Backend: "native-policy-workspace", Reasons: []string{"native process/filesystem policy adapter is planned", "WSL2 is compatibility-only and does not protect the Windows host"}}
	default:
		return Capability{Platform: runtime.GOOS, Backend: "unsupported", Reasons: []string{"no supported transaction backend for this operating system"}}
	}
}

func (c Capability) ValidateForRun() error {
	if !c.Supported {
		return fmt.Errorf("platform backend %s is unavailable: %v", c.Platform, c.Reasons)
	}
	return nil
}

func NativeBackend() Backend {
	if runtime.GOOS == "darwin" {
		return NewMacOSBackend()
	}
	return UnavailableBackend{Report: Probe()}
}
