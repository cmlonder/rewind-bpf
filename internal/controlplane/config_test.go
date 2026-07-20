package controlplane

import (
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
)

func testPolicy() policy.Policy {
	return policy.Policy{
		Read:    policy.ReadPolicy{Mode: policy.ModeAudit},
		Write:   policy.WritePolicy{Mode: "rollback", Scope: "workspace"},
		Network: policy.NetworkPolicy{Mode: policy.ModeOff},
	}
}

func TestStoreCreatesPolicyAndAssignsWorkspaceAtomically(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "config.json"))
	if err := store.CreatePolicy(PolicyPackage{Name: "strict-agent", Version: "1.0.0", Policy: testPolicy()}); err != nil {
		t.Fatal(err)
	}
	if err := store.AssignWorkspace(Workspace{Name: "payments-api", Path: "/workspaces/payments-api", Policy: "strict-agent@1.0.0"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 2 || len(snapshot.Policies) != 1 || len(snapshot.Workspaces) != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestStoreRejectsUnknownPolicyAndDuplicatePackage(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "config.json"))
	value := PolicyPackage{Name: "safe", Version: "1.0.0", Policy: testPolicy()}
	if err := store.CreatePolicy(value); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePolicy(value); err == nil {
		t.Fatal("duplicate policy unexpectedly accepted")
	}
	if err := store.AssignWorkspace(Workspace{Name: "scratch", Path: "/workspaces/scratch", Policy: "missing@1.0.0"}); err == nil {
		t.Fatal("unknown policy unexpectedly accepted")
	}
}

func TestStoreRejectsUnsafeWorkspace(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "config.json"))
	if err := store.AssignWorkspace(Workspace{Name: "bad name", Path: "/workspaces/bad", Policy: "none"}); err == nil {
		t.Fatal("invalid workspace name unexpectedly accepted")
	}
	if err := store.AssignWorkspace(Workspace{Name: "root", Path: "/", Policy: "none"}); err == nil {
		t.Fatal("root workspace unexpectedly accepted")
	}
}
