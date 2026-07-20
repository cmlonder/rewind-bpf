// Package platform describes host backends without silently downgrading a
// protected run. The report is safe to call on any OS and performs no setup.
package platform

import (
	"fmt"
	"runtime"
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

func Probe() Capability {
	switch runtime.GOOS {
	case "linux":
		return Capability{Platform: "linux", Backend: "overlayfs-landlock-ebpf", Supported: true, ProjectIsolation: true, SensitiveReadDeny: true, Reasons: []string{"requires runtime capability probe for the selected kernel backend"}}
	case "darwin":
		return Capability{Platform: "darwin", Backend: "seatbelt-apfs", Reasons: []string{"native Seatbelt/EndpointSecurity adapter is planned", "APFS disposable workspace adapter is not enabled"}}
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
	return UnavailableBackend{Report: Probe()}
}
