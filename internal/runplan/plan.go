// Package runplan composes the safe, pre-execution inputs for one protected
// agent run. It does not mount filesystems, start processes, or attach eBPF.
package runplan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/agent"
	"github.com/rewindbpf/rewind/internal/capabilities"
	"github.com/rewindbpf/rewind/internal/landlock"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/netpolicy"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/pii"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policycompile"
)

type Config struct {
	Workspace         string
	RuntimeRoot       string
	Policy            policy.Policy
	RuntimeRoots      []string
	OverlayBackend    overlay.Backend
	NetworkBackend    string
	AgentAdapter      string
	CheckpointGraph   string
	CheckpointID      string
	CheckpointParents []string
}

type Plan struct {
	Run               lifecycle.Run           `json:"run"`
	Layout            overlay.Layout          `json:"layout"`
	Manifest          manifest.Manifest       `json:"manifest"`
	ReadRules         policycompile.ReadRules `json:"read_rules"`
	Landlock          *landlock.Plan          `json:"landlock,omitempty"`
	Resources         policy.ResourcePolicy   `json:"resources,omitempty"`
	OverlayBackend    overlay.Backend         `json:"overlay_backend"`
	CgroupPath        string                  `json:"cgroup_path,omitempty"`
	Capabilities      capabilities.Report     `json:"capabilities,omitempty"`
	HistoryPath       string                  `json:"history_path,omitempty"`
	Network           netpolicy.Plan          `json:"network"`
	AgentAdapter      agent.Kind              `json:"agent_adapter"`
	AgentHookProtocol string                  `json:"agent_hook_protocol,omitempty"`
	AgentExecutables  []string                `json:"agent_executables,omitempty"`
	PIIFindings       []pii.Finding           `json:"pii_findings,omitempty"`
	PIIMode           policy.Mode             `json:"pii_mode,omitempty"`
	CheckpointGraph   string                  `json:"checkpoint_graph,omitempty"`
	CheckpointID      string                  `json:"checkpoint_id,omitempty"`
	CheckpointParents []string                `json:"checkpoint_parents,omitempty"`
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
	agentSpec, err := agent.Resolve(config.AgentAdapter)
	if err != nil {
		return Plan{}, fmt.Errorf("build run plan: %w", err)
	}
	networkPlan, err := netpolicy.Compile(config.Policy.Network)
	if err != nil {
		return Plan{}, fmt.Errorf("build run plan: network policy: %w", err)
	}
	if networkPlan.Mode == policy.ModeEnforce && config.NetworkBackend != netpolicy.BackendProxy && config.NetworkBackend != netpolicy.BackendDeny && config.NetworkBackend != netpolicy.BackendNS {
		return Plan{}, fmt.Errorf("build run plan: network enforce requires --network-backend proxy, deny, or namespace")
	}
	if config.NetworkBackend == netpolicy.BackendDeny && len(networkPlan.AllowDomains) > 0 {
		return Plan{}, fmt.Errorf("build run plan: %s network backend cannot provide allow_domains; use proxy backend", config.NetworkBackend)
	}
	// Keep the defense decision in the inert plan that is persisted before
	// process start. This makes the run record auditable and prevents the
	// coordinator from silently changing the security posture after planning.
	networkPlan.RawSocketDeny = networkPlan.Mode == policy.ModeEnforce && (config.NetworkBackend == netpolicy.BackendProxy || config.NetworkBackend == netpolicy.BackendDeny)
	networkPlan.NetworkDeny = networkPlan.Mode == policy.ModeEnforce && config.NetworkBackend == netpolicy.BackendDeny
	networkPlan.NetworkNS = networkPlan.Mode == policy.ModeEnforce && config.NetworkBackend == netpolicy.BackendNS
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
	piiFindings, err := piiFindingsForPlan(config.Policy.Read.PII, workspace)
	if err != nil {
		return Plan{}, err
	}
	readPolicy := config.Policy.Read
	if readPolicy.PII.Mode == policy.ModeEnforce {
		for _, finding := range piiFindings {
			relative, err := filepath.Rel(workspace, finding.Path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return Plan{}, fmt.Errorf("build run plan: PII finding escapes workspace: %s", finding.Path)
			}
			readPolicy.Deny = append(readPolicy.Deny, filepath.ToSlash(filepath.Join(layout.Merged, relative)))
		}
	}
	readRules, err := policycompile.CompileRead(readPolicy, layout.Merged, snapshot)
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
	checkpointID := strings.TrimSpace(config.CheckpointID)
	if checkpointID == "" && strings.TrimSpace(config.CheckpointGraph) != "" {
		checkpointID = run.ID
	}
	return Plan{Run: run, Layout: layout, Manifest: snapshot, ReadRules: readRules, Landlock: landlockPlan, Resources: config.Policy.Resources, OverlayBackend: backend, Network: networkPlan, AgentAdapter: agentSpec.Kind, AgentHookProtocol: agentSpec.HookProtocol, AgentExecutables: append([]string(nil), agentSpec.Executables...), PIIFindings: piiFindings, PIIMode: config.Policy.Read.PII.Mode, CheckpointGraph: config.CheckpointGraph, CheckpointID: checkpointID, CheckpointParents: append([]string(nil), config.CheckpointParents...)}, nil
}

func piiFindingsForPlan(config policy.PIIPolicy, workspace string) ([]pii.Finding, error) {
	if config.Mode == "" || config.Mode == policy.ModeOff {
		return nil, nil
	}
	findings, err := pii.ScanPath(workspace)
	if err != nil {
		return nil, fmt.Errorf("build run plan: PII scan: %w", err)
	}
	if config.Mode == policy.ModeAudit || config.Mode == policy.ModeEnforce {
		return findings, nil
	}
	return nil, fmt.Errorf("build run plan: unsupported PII mode %q", config.Mode)
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
