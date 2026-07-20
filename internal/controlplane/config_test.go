package controlplane

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policybundle"
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

func TestStorePersistsWorkspaceAgentAdapter(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "config.json"))
	if err := store.AssignWorkspace(Workspace{Name: "demo", Path: "/workspaces/demo", Policy: "none", Adapter: "codex"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].Adapter != "codex" {
		t.Fatalf("workspaces=%+v", snapshot.Workspaces)
	}
}

func TestStoreRejectsUnknownWorkspaceAgentAdapter(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "config.json"))
	if err := store.AssignWorkspace(Workspace{Name: "demo", Path: "/workspaces/demo", Policy: "none", Adapter: "unknown"}); err == nil {
		t.Fatal("expected unknown adapter error")
	}
}

func TestStoreImportsVerifiedSignedPolicy(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "signed-agent", Version: "1.0.0", Description: "verified", Policy: testPolicy()}, private)
	if err != nil {
		t.Fatal(err)
	}
	store := Open(filepath.Join(t.TempDir(), "config.json"))
	if err := store.CreateSignedPolicy(signed); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Policies) != 1 || !snapshot.Policies[0].Signed || snapshot.Policies[0].Name != "signed-agent" || snapshot.Policies[0].SignerKeyID != signed.KeyID {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	signed.Signature = "invalid"
	if err := store.CreateSignedPolicy(signed); err == nil {
		t.Fatal("tampered signed policy unexpectedly accepted")
	}
}

func TestStoreRejectsTamperedPersistedSignedEnvelope(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "durable-agent", Version: "1.0.0", Policy: testPolicy()}, private)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	store := Open(path)
	if err := store.CreateSignedPolicy(signed); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Policies[0].SignedBundle.Signature = "tampered"
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(); err == nil {
		t.Fatal("tampered persisted envelope unexpectedly accepted")
	}
}
