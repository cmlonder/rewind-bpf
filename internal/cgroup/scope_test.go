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
