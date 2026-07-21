//go:build darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/acceptance"
	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

// MacOSBackend provides a safe native filesystem transaction without claiming
// EndpointSecurity telemetry. On APFS it uses clonefile-backed cp -c, so the
// initial snapshot is copy-on-write; a missing clone capability fails closed.
type MacOSBackend struct{}

func NewMacOSBackend() MacOSBackend { return MacOSBackend{} }

func (MacOSBackend) Capabilities() Capability {
	return Capability{
		Platform:             "darwin",
		Backend:              "apfs-clone-seatbelt",
		Supported:            true,
		ProjectIsolation:     true,
		SensitiveReadDeny:    true,
		NetworkEgressControl: false,
		Reasons:              []string{"filesystem transaction and Seatbelt read policy are available", "EndpointSecurity telemetry and network enforcement require the signed helper gate"},
	}
}

// Prepare creates a transaction under an explicit runtime root. The runtime
// root must be disposable and must not contain the source workspace.
func (MacOSBackend) PrepareAt(ctx context.Context, workspace, runtimeRoot string) (*MacOSTransaction, error) {
	source, err := resolveDirectory(workspace, "workspace")
	if err != nil {
		return nil, err
	}
	runtimeRoot, err = filepath.Abs(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve macOS runtime root: %w", err)
	}
	runtimeRoot = filepath.Clean(runtimeRoot)
	if runtimeRoot == "/" || within(source, runtimeRoot) || within(runtimeRoot, source) {
		return nil, fmt.Errorf("macOS runtime root and workspace must be separate non-root directories")
	}
	if info, statErr := os.Lstat(runtimeRoot); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("macOS runtime root must be a real directory: %s", runtimeRoot)
		}
		entries, readErr := os.ReadDir(runtimeRoot)
		if readErr != nil {
			return nil, fmt.Errorf("inspect macOS runtime root: %w", readErr)
		}
		if len(entries) != 0 {
			return nil, fmt.Errorf("macOS runtime root must be empty and disposable: %s", runtimeRoot)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("inspect macOS runtime root: %w", statErr)
	} else if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create macOS runtime root: %w", err)
	}
	view := filepath.Join(runtimeRoot, "view")
	if err := os.MkdirAll(view, 0o700); err != nil {
		return nil, fmt.Errorf("create macOS view: %w", err)
	}
	if err := cloneWorkspace(ctx, source, view); err != nil {
		_ = os.RemoveAll(runtimeRoot)
		return nil, err
	}
	base, err := manifest.Build(source)
	if err != nil {
		_ = os.RemoveAll(runtimeRoot)
		return nil, fmt.Errorf("build macOS base manifest: %w", err)
	}
	return &MacOSTransaction{workspace: source, runtimeRoot: runtimeRoot, view: view, base: base}, nil
}

// Prepare satisfies the portable backend interface by allocating an isolated
// temporary runtime. Callers that need durable records should use PrepareAt.
func (b MacOSBackend) Prepare(ctx context.Context, workspace string) (Transaction, error) {
	runtimeRoot, err := os.MkdirTemp("", "rewind-macos-run-")
	if err != nil {
		return nil, fmt.Errorf("create macOS transaction root: %w", err)
	}
	tx, err := b.PrepareAt(ctx, workspace, runtimeRoot)
	if err != nil {
		_ = os.RemoveAll(runtimeRoot)
		return nil, err
	}
	return tx, nil
}

type MacOSTransaction struct {
	workspace   string
	runtimeRoot string
	view        string
	base        manifest.Manifest
	discarded   bool
	accepted    bool
}

func (t *MacOSTransaction) View() string { return t.view }

func (t *MacOSTransaction) Diff(context.Context) (any, error) {
	if t == nil || t.discarded || t.accepted {
		return nil, fmt.Errorf("macOS transaction is no longer available")
	}
	after, err := manifest.Build(t.view)
	if err != nil {
		return nil, err
	}
	return diff.Compare(t.base, after), nil
}

func (t *MacOSTransaction) Discard(context.Context) error {
	if t == nil || t.discarded || t.accepted {
		return nil
	}
	if err := os.RemoveAll(t.runtimeRoot); err != nil {
		return fmt.Errorf("discard macOS transaction: %w", err)
	}
	t.discarded = true
	return nil
}

func (t *MacOSTransaction) Recover(ctx context.Context) error { return t.Discard(ctx) }

func (t *MacOSTransaction) Accept(_ context.Context, destination string) error {
	if t == nil || t.discarded || t.accepted {
		return fmt.Errorf("macOS transaction is no longer available")
	}
	destination, err := resolveDirectory(destination, "destination")
	if err != nil {
		return err
	}
	if destination != t.workspace {
		return fmt.Errorf("macOS transaction destination must equal its source workspace")
	}
	current, err := manifest.Build(destination)
	if err != nil {
		return err
	}
	candidate, err := manifest.Build(t.view)
	if err != nil {
		return err
	}
	if _, err := acceptance.Apply(t.base, current, candidate, t.view, destination); err != nil {
		return fmt.Errorf("accept macOS transaction: %w", err)
	}
	if err := os.RemoveAll(t.runtimeRoot); err != nil {
		return fmt.Errorf("cleanup accepted macOS transaction: %w", err)
	}
	t.accepted = true
	return nil
}

// DiscardMacOSRuntime is the idempotent rollback operation used by the CLI
// after a process has exited. It validates that the runtime is separate from
// the workspace before removing only the dedicated transaction directory.
func DiscardMacOSRuntime(runtimeRoot, workspace string) error {
	runtimeRoot, err := filepath.Abs(runtimeRoot)
	if err != nil {
		return fmt.Errorf("resolve macOS runtime root: %w", err)
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("resolve macOS workspace: %w", err)
	}
	runtimeRoot = filepath.Clean(runtimeRoot)
	workspace = filepath.Clean(workspace)
	if runtimeRoot == "/" || within(runtimeRoot, workspace) || within(workspace, runtimeRoot) {
		return fmt.Errorf("refusing unsafe macOS runtime cleanup")
	}
	if info, statErr := os.Lstat(runtimeRoot); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked macOS runtime cleanup")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect macOS runtime cleanup: %w", statErr)
	}
	return os.RemoveAll(runtimeRoot)
}

// AcceptMacOSRuntime reconstructs a transaction from a persisted record and
// applies it through the same conflict-checked acceptance path as a live run.
func AcceptMacOSRuntime(ctx context.Context, workspace, runtimeRoot string, base manifest.Manifest) ([]diff.Change, error) {
	workspacePath, err := resolveDirectory(workspace, "workspace")
	if err != nil {
		return nil, err
	}
	runtimeRoot, err = filepath.Abs(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve macOS runtime root: %w", err)
	}
	runtimeRoot = filepath.Clean(runtimeRoot)
	if runtimeRoot == "/" || within(runtimeRoot, workspacePath) || within(workspacePath, runtimeRoot) {
		return nil, fmt.Errorf("refusing unsafe macOS runtime acceptance")
	}
	if info, statErr := os.Lstat(runtimeRoot); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing symlinked macOS runtime acceptance")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("inspect macOS runtime acceptance: %w", statErr)
	}
	view := filepath.Join(runtimeRoot, "view")
	if _, err := os.Stat(view); err != nil {
		return nil, fmt.Errorf("macOS staged view is unavailable: %w", err)
	}
	tx := &MacOSTransaction{workspace: workspacePath, runtimeRoot: runtimeRoot, view: view, base: base}
	candidate, err := manifest.Build(view)
	if err != nil {
		return nil, err
	}
	changes := diff.Compare(base, candidate)
	if err := tx.Accept(ctx, workspacePath); err != nil {
		return changes, err
	}
	return changes, nil
}

func cloneWorkspace(ctx context.Context, source, destination string) error {
	// Keep the trailing `/.`: filepath.Join cleans it away and macOS cp would
	// then copy the workspace directory itself as a nested child of view.
	sourceContents := source + string(filepath.Separator) + "."
	command := exec.CommandContext(ctx, "/bin/cp", "-c", "-R", sourceContents, destination)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("APFS clone workspace: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func resolveDirectory(value, label string) (string, error) {
	path, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("resolve macOS %s: %w", label, err)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("macOS %s must be an existing directory: %s", label, path)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = resolved
	} else {
		return "", fmt.Errorf("resolve macOS %s symlinks: %w", label, resolveErr)
	}
	return path, nil
}

func within(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
