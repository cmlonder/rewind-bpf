package overlay

import (
	"context"
	"fmt"
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

func TestLayoutCanUseAnExplicitWorkspaceAsLower(t *testing.T) {
	runtimeRoot := filepath.Join(t.TempDir(), "run")
	workspace := filepath.Join(t.TempDir(), "workspace")
	layout, err := NewLayoutWithLower(runtimeRoot, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Lower != workspace {
		t.Fatalf("lower = %q, want %q", layout.Lower, workspace)
	}
	if _, err := NewLayoutWithLower(filepath.Join(workspace, "run"), workspace); err == nil {
		t.Fatal("expected runtime root inside lower path to be rejected")
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
	if strings.Contains(command, "override_creds") {
		t.Fatalf("kernel backend must not pass unsupported override_creds option: %q", command)
	}
	if _, err := os.Stat(layout.Merged); err != nil {
		t.Fatalf("prepare should create merged directory: %v", err)
	}
}

type fakeMountProcess struct{}

func (fakeMountProcess) Wait() error { return nil }
func (fakeMountProcess) Kill() error { return nil }

type fakeProcessStarter struct {
	command []string
}

func (f *fakeProcessStarter) Start(_ context.Context, command string, args ...string) (MountProcess, error) {
	f.command = append([]string{command}, args...)
	return fakeMountProcess{}, nil
}

func TestFuseMountBuildsAgentOwnedCommand(t *testing.T) {
	runner := &fakeRunner{}
	starter := &fakeProcessStarter{}
	layout, err := NewLayoutWithLower(filepath.Join(t.TempDir(), "run"), filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{
		Runner:  runner,
		Starter: starter,
		Backend: BackendFuse,
		Owner:   &Owner{UID: os.Getuid(), GID: os.Getgid()},
	}
	if err := manager.Mount(context.Background(), layout); err != nil {
		t.Fatal(err)
	}
	command := strings.Join(starter.command, " ")
	for _, expected := range []string{"fuse-overlayfs", "-f", "lowerdir=" + layout.Lower, "upperdir=" + layout.Upper, "workdir=" + layout.Work, fmt.Sprintf("uid=%d", os.Getuid()), fmt.Sprintf("gid=%d", os.Getgid()), "allow_other", layout.Merged} {
		if !strings.Contains(command, expected) {
			t.Errorf("command %q does not contain %q", command, expected)
		}
	}
	if len(runner.commands) == 0 || runner.commands[0][0] != "mountpoint" {
		t.Fatalf("fuse mount should wait for mountpoint readiness, got %#v", runner.commands)
	}
}

func TestFuseUnmountUsesFusermount(t *testing.T) {
	runner := &fakeRunner{}
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Runner: runner, Backend: BackendFuse}
	if err := manager.Unmount(context.Background(), layout); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 || runner.commands[0][0] != "fusermount3" {
		t.Fatalf("got %#v, want fusermount3", runner.commands)
	}
}

type settlingRunner struct {
	calls int
}

func (r *settlingRunner) Run(_ context.Context, command string, args ...string) error {
	r.calls++
	if command != "fusermount3" || len(args) != 2 || args[0] != "-u" {
		return fmt.Errorf("unexpected unmount command %s %v", command, args)
	}
	if r.calls < 3 {
		return fmt.Errorf("fusermount3: device or resource busy")
	}
	return nil
}

func TestFuseUnmountWaitsForBusyMountToSettle(t *testing.T) {
	runner := &settlingRunner{}
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Runner: runner, Backend: BackendFuse}
	if err := manager.Unmount(context.Background(), layout); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 3 {
		t.Fatalf("unmount attempts=%d, want 3", runner.calls)
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

func TestFuseRollbackAcceptsMountAlreadyGoneAfterCrash(t *testing.T) {
	runner := &fakeRunner{err: fmt.Errorf("fusermount3: invalid argument")}
	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Upper, "temporary"), []byte("change"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Runner: runner, Backend: BackendFuse}
	if err := manager.Rollback(context.Background(), layout); err != nil {
		t.Fatalf("expected already-unmounted recovery to succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.Upper, "temporary")); !os.IsNotExist(err) {
		t.Fatalf("upper layer was not discarded: %v", err)
	}
}
