package cgroup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAtCreatesSanitizedScope(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory"), 0o644); err != nil {
		t.Fatal(err)
	}
	scope, err := NewAt(root, "run/unsafe id")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(scope.Path()) != "run-run-unsafe-id" {
		t.Fatalf("scope path = %q", scope.Path())
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewAtRequiresControllers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAt(root, "run-1"); err == nil {
		t.Fatal("expected cgroup controller error")
	}
}

func TestCloseWaitsForTerminatedMembers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu"), 0o644); err != nil {
		t.Fatal(err)
	}
	scope, err := NewAt(root, "run-drain")
	if err != nil {
		t.Fatal(err)
	}
	procs := filepath.Join(scope.Path(), "cgroup.procs")
	if err := os.WriteFile(procs, []byte("123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = os.Remove(procs)
	}()
	if err := scope.Close(); err != nil {
		t.Fatalf("close should wait for members to drain: %v", err)
	}
	if _, err := os.Stat(scope.Path()); !os.IsNotExist(err) {
		t.Fatalf("scope remains after close: %v", err)
	}
}

func TestScopeConfigureWritesRequestedLimits(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory pids"), 0o644); err != nil {
		t.Fatal(err)
	}
	scope, err := NewAt(root, "run_limits")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pids.max", "memory.max", "cpu.max"} {
		if err := os.WriteFile(filepath.Join(scope.Path(), name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := scope.Configure(Limits{PIDsMax: "128", MemoryMax: "268435456", CPUMax: "50000 100000"}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"pids.max": "128\n", "memory.max": "268435456\n", "cpu.max": "50000 100000\n"} {
		got, err := os.ReadFile(filepath.Join(scope.Path(), name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"pids.max", "memory.max", "cpu.max"} {
		if err := os.Remove(filepath.Join(scope.Path(), name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewAtWithLimitsDelegatesControllers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory pids"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "rewind")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "cgroup.subtree_control"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scope, err := NewAtWithLimits(root, "run_delegate", Limits{PIDsMax: "64", MemoryMax: "1000", CPUMax: "1 100000"})
	if err != nil {
		t.Fatal(err)
	}
	delegated, err := os.ReadFile(filepath.Join(parent, "cgroup.subtree_control"))
	if err != nil {
		t.Fatal(err)
	}
	if string(delegated) != "+pids +memory +cpu\n" {
		t.Fatalf("delegated controllers = %q", delegated)
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
}
