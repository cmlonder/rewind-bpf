package landlock

import (
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policycompile"
)

func TestBuildPlanKeepsOnlyEnforceAllowlist(t *testing.T) {
	plan, err := BuildPlan("/workspace", policycompile.ReadRules{
		Mode:         policy.ModeEnforce,
		AllowedFiles: []string{"/workspace/z.txt", "/workspace/a.txt"},
		AllowedDirs:  []string{"/workspace/src"},
	}, []string{"/usr", "/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plan.AllowedFiles[0], "/workspace/a.txt"; got != want {
		t.Fatalf("first allowed file = %q, want %q", got, want)
	}
	if got, want := plan.RuntimeRoots[0], "/bin"; got != want {
		t.Fatalf("first runtime root = %q, want %q", got, want)
	}
}

func TestBuildPlanRejectsNonEnforceAndEscapes(t *testing.T) {
	if _, err := BuildPlan("/workspace", policycompile.ReadRules{Mode: policy.ModeAudit}, nil); err == nil {
		t.Fatal("expected audit mode rejection")
	}
	if _, err := BuildPlan("/workspace", policycompile.ReadRules{
		Mode:         policy.ModeEnforce,
		AllowedFiles: []string{"/workspace/../secret"},
	}, nil); err == nil {
		t.Fatal("expected escaped path rejection")
	}
}
