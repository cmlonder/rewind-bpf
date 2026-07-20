package runplan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
)

func TestBuildComposesOverlayManifestAndLandlockPlan(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "public.txt"), []byte("public\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("synthetic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(t.TempDir(), "run")

	plan, err := Build(Config{
		Workspace:   workspace,
		RuntimeRoot: runtimeRoot,
		Policy: policy.Policy{Read: policy.ReadPolicy{
			Mode: policy.ModeEnforce,
			Deny: []string{"**/.env"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Run.State != "preparing" {
		t.Fatalf("run state = %s", plan.Run.State)
	}
	if plan.OverlayBackend != "fuse" {
		t.Fatalf("overlay backend = %q, want fuse", plan.OverlayBackend)
	}
	if plan.Layout.Lower != workspace {
		t.Fatalf("lower = %q, want %q", plan.Layout.Lower, workspace)
	}
	if len(plan.Manifest.Entries) != 2 {
		t.Fatalf("manifest entries = %d, want 2", len(plan.Manifest.Entries))
	}
	if plan.Landlock == nil || len(plan.Landlock.AllowedFiles) != 1 {
		t.Fatalf("Landlock plan = %+v, want one allowed file", plan.Landlock)
	}
	if len(plan.ReadRules.Rules) != 1 || plan.ReadRules.Rules[0].Decision != "deny" {
		t.Fatalf("read rules = %+v", plan.ReadRules.Rules)
	}
}

func TestBuildRejectsWorkspaceRuntimeOverlap(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Build(Config{Workspace: workspace, RuntimeRoot: filepath.Join(workspace, "run")}); err == nil {
		t.Fatal("expected workspace/runtime overlap error")
	}
}

func TestBuildFailsClosedForUnavailableNetworkEnforcement(t *testing.T) {
	workspace := t.TempDir()
	_, err := Build(Config{
		Workspace:   workspace,
		RuntimeRoot: filepath.Join(t.TempDir(), "run"),
		Policy:      policy.Policy{Network: policy.NetworkPolicy{Mode: policy.ModeEnforce}},
	})
	if err == nil {
		t.Fatal("expected network enforcement to fail closed")
	}
}

func TestBuildAcceptsExplicitProxyNetworkBackend(t *testing.T) {
	workspace := t.TempDir()
	plan, err := Build(Config{Workspace: workspace, RuntimeRoot: filepath.Join(t.TempDir(), "run"), NetworkBackend: "proxy", Policy: policy.Policy{Network: policy.NetworkPolicy{Mode: policy.ModeEnforce, AllowDomains: []string{"example.com"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Network.Mode != policy.ModeEnforce || len(plan.Network.AllowDomains) != 1 {
		t.Fatalf("network plan=%+v", plan.Network)
	}
	if !plan.Network.RawSocketDeny {
		t.Fatalf("network plan=%+v, want raw socket defense enabled", plan.Network)
	}
	encoded, err := json.Marshal(plan.Network)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"raw_socket_deny":true`) {
		t.Fatalf("serialized network plan=%s, want raw_socket_deny=true", encoded)
	}
}

func TestBuildAcceptsExplicitProxyForAuditMode(t *testing.T) {
	workspace := t.TempDir()
	plan, err := Build(Config{
		Workspace:      workspace,
		RuntimeRoot:    filepath.Join(t.TempDir(), "run"),
		NetworkBackend: "proxy",
		Policy: policy.Policy{Network: policy.NetworkPolicy{
			Mode:         policy.ModeAudit,
			AllowDomains: []string{"example.com"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Network.Mode != policy.ModeAudit || len(plan.Network.AllowDomains) != 1 {
		t.Fatalf("plan=%+v", plan.Network)
	}
	if plan.Network.RawSocketDeny {
		t.Fatalf("plan=%+v, want raw socket defense disabled in audit mode", plan.Network)
	}
}
