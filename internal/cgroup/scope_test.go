package cgroup

import (
	"os"
	"path/filepath"
	"testing"
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
