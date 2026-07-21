//go:build darwin

package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/diff"
)

func TestMacOSBackendPrepareDiffAndDiscard(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "marker.txt"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	tx, err := NewMacOSBackend().PrepareAt(context.Background(), workspace, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	viewMarker := filepath.Join(tx.View(), "marker.txt")
	if got, err := os.ReadFile(viewMarker); err != nil || string(got) != "original\n" {
		t.Fatalf("staged marker=%q err=%v", got, err)
	}
	if err := os.WriteFile(viewMarker, []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changes, err := tx.Diff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if typed, ok := changes.([]diff.Change); !ok || len(typed) == 0 {
		t.Fatal("expected a staged change")
	}
	if err := tx.Discard(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runtimeRoot); !os.IsNotExist(err) {
		t.Fatalf("runtime root still exists: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "marker.txt")); err != nil || string(got) != "original\n" {
		t.Fatalf("workspace marker=%q err=%v", got, err)
	}
}
