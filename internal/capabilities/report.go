// Package capabilities reports kernel and userspace prerequisites without
// mounting filesystems, loading eBPF, or changing host state.
package capabilities

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Report struct {
	OS            string   `json:"os"`
	Arch          string   `json:"arch"`
	KernelRelease string   `json:"kernel_release,omitempty"`
	OverlayFS     bool     `json:"overlayfs"`
	FuseOverlayFS bool     `json:"fuse_overlayfs"`
	BTF           bool     `json:"btf"`
	Landlock      bool     `json:"landlock"`
	BPFLSM        bool     `json:"bpf_lsm"`
	CgroupV2      bool     `json:"cgroup_v2"`
	Seccomp       bool     `json:"seccomp"`
	NetworkNS     bool     `json:"network_namespace"`
	Warnings      []string `json:"warnings,omitempty"`
}

func Probe() Report {
	report := Report{OS: runtime.GOOS, Arch: runtime.GOARCH}
	report.KernelRelease = readTrimmed("/proc/sys/kernel/osrelease")
	report.OverlayFS = contains(readTrimmed("/proc/filesystems"), "overlay")
	report.BTF = exists("/sys/kernel/btf/vmlinux")
	lsm := readTrimmed("/sys/kernel/security/lsm")
	report.Landlock = containsWord(lsm, "landlock")
	report.BPFLSM = containsWord(lsm, "bpf")
	report.CgroupV2 = exists("/sys/fs/cgroup/cgroup.controllers") && readTrimmed("/sys/fs/cgroup/cgroup.controllers") != ""
	if _, err := exec.LookPath("fuse-overlayfs"); err == nil {
		report.FuseOverlayFS = true
	}
	if _, err := exec.LookPath("unshare"); err == nil {
		report.NetworkNS = true
	}
	// The Seccomp field in /proc/self/status only reports this process' current
	// mode (usually 0); it is not a capability probe. The actions_avail kernel
	// interface is the read-only capability signal we need before installing a
	// filter in the agent helper.
	report.Seccomp = exists("/proc/sys/kernel/seccomp/actions_avail")
	if !report.OverlayFS && !report.FuseOverlayFS {
		report.Warnings = append(report.Warnings, "no OverlayFS or fuse-overlayfs backend detected")
	}
	if !report.Landlock && !report.BPFLSM {
		report.Warnings = append(report.Warnings, "no filesystem enforcement LSM detected")
	}
	if !report.CgroupV2 {
		report.Warnings = append(report.Warnings, "cgroup-v2 is unavailable; process scope would be PID fallback")
	}
	return report
}

func (r Report) ValidateForProtectedRun(backend string, enforceRead, denyRawNetwork bool) error {
	if backend == "kernel" && !r.OverlayFS {
		return fmt.Errorf("kernel OverlayFS backend is unavailable")
	}
	if backend == "fuse" && !r.FuseOverlayFS {
		return fmt.Errorf("fuse-overlayfs backend is unavailable")
	}
	if enforceRead && !r.Landlock && !r.BPFLSM {
		return fmt.Errorf("read enforcement requested but Landlock and BPF-LSM are unavailable")
	}
	if denyRawNetwork && !r.Seccomp {
		return fmt.Errorf("raw-socket network enforcement requested but seccomp is unavailable")
	}
	if !r.CgroupV2 {
		return fmt.Errorf("protected run requires cgroup-v2 process scope")
	}
	return nil
}

func (r Report) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

func readTrimmed(path string) string {
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func contains(value, needle string) bool { return strings.Contains(value, needle) }

func containsWord(value, needle string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.TrimSpace(item) == needle {
			return true
		}
	}
	return false
}
