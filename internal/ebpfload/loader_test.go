package ebpfload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsUnsafeInputsBeforeKernelOperations(t *testing.T) {
	if _, err := Load("", "run_test", 42); err == nil {
		t.Fatal("expected empty object path error")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.o"), "run_test", 42); err == nil {
		t.Fatal("expected missing object error")
	}
	if _, err := Load(t.TempDir(), "run_test", 42); err == nil {
		t.Fatal("expected directory object error")
	}
	objectPath := filepath.Join(t.TempDir(), "invalid.o")
	if err := os.WriteFile(objectPath, []byte("not an ELF"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(objectPath, "run_test", 42); err == nil {
		t.Fatal("expected invalid object error")
	}
}

func TestLoadRequiresRunAndTargetScope(t *testing.T) {
	objectPath := filepath.Join(t.TempDir(), "object.o")
	if err := os.WriteFile(objectPath, []byte("not an ELF"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(objectPath, "", 42); err == nil {
		t.Fatal("expected empty run id error")
	}
	if _, err := Load(objectPath, "run_test", 0); err == nil {
		t.Fatal("expected zero target pid error")
	}
}
