package overlay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	commands [][]string
	err      error
}

func (f *fakeRunner) Run(_ context.Context, command string, args ...string) error {
	f.commands = append(f.commands, append([]string{command}, args...))
	return f.err
}

func TestNewLayoutKeepsAllPathsInsideRuntimeRoot(t *testing.T) {
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{layout.Lower, layout.Upper, layout.Work, layout.Merged} {
		rel, err := filepath.Rel(layout.Root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("path escapes root: %s", path)
		}
	}
}

func TestLayoutRejectsUnsafeRootsAndMountCharacters(t *testing.T) {
	if _, err := NewLayout("/"); err == nil {
		t.Fatal("expected root filesystem to be rejected")
	}
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	layout.Lower = filepath.Join(layout.Root, "lower,unsafe")
	if err := layout.Validate(); err == nil {
		t.Fatal("expected comma in mount path to be rejected")
	}
}

func TestMountBuildsExpectedCommandWithoutExecutingIt(t *testing.T) {
	runner := &fakeRunner{}
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Runner: runner}
	if err := manager.Mount(context.Background(), layout); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(runner.commands))
	}
	command := strings.Join(runner.commands[0], " ")
	for _, expected := range []string{"mount", "-t overlay", "lowerdir=" + layout.Lower, "upperdir=" + layout.Upper, "workdir=" + layout.Work, layout.Merged} {
		if !strings.Contains(command, expected) {
			t.Errorf("command %q does not contain %q", command, expected)
		}
	}
	if _, err := os.Stat(layout.Merged); err != nil {
		t.Fatalf("prepare should create merged directory: %v", err)
	}
}

func TestRollbackDoesNotRemoveLowerAndRequiresUnmount(t *testing.T) {
	runner := &fakeRunner{}
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Lower, "marker"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Upper, "change"), []byte("temporary"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Runner: runner}
	if err := manager.Rollback(context.Background(), layout); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.Lower, "marker")); err != nil {
		t.Fatalf("lower file was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.Upper, "change")); !os.IsNotExist(err) {
		t.Fatalf("upper file still exists or unexpected error: %v", err)
	}
	if len(runner.commands) != 1 || runner.commands[0][0] != "umount" {
		t.Fatalf("rollback should unmount first, got %#v", runner.commands)
	}
}
