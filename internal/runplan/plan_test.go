package runplan

import (
	"os"
	"path/filepath"
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
