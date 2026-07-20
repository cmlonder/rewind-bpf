package overlay

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Layout describes the directories owned by one RewindBPF filesystem run.
// The root must be a dedicated runtime directory, never a user home or host
// filesystem root.
type Layout struct {
	Root   string
	Lower  string
	Upper  string
	Work   string
	Merged string
}

func NewLayout(root string) (Layout, error) {
	if strings.TrimSpace(root) == "" {
		return Layout{}, fmt.Errorf("overlay root cannot be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, fmt.Errorf("resolve overlay root: %w", err)
	}
	root = filepath.Clean(abs)
	if root == string(filepath.Separator) {
		return Layout{}, fmt.Errorf("refusing filesystem root as overlay runtime")
	}
	layout := Layout{
		Root:   root,
		Lower:  filepath.Join(root, "lower"),
		Upper:  filepath.Join(root, "upper"),
		Work:   filepath.Join(root, "work"),
		Merged: filepath.Join(root, "merged"),
	}
	if err := layout.Validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// NewLayoutWithLower creates a runtime layout whose lower layer is an
// explicitly supplied immutable workspace. Keeping the original workspace as
// lowerdir avoids an upfront copy; rollback only removes upper/work. The
// caller must still provide a disposable or otherwise protected workspace.
func NewLayoutWithLower(root, lower string) (Layout, error) {
	layout, err := NewLayout(root)
	if err != nil {
		return Layout{}, err
	}
	if strings.TrimSpace(lower) == "" {
		return Layout{}, fmt.Errorf("overlay lower path cannot be empty")
	}
	absLower, err := filepath.Abs(lower)
	if err != nil {
		return Layout{}, fmt.Errorf("resolve overlay lower path: %w", err)
	}
	layout.Lower = filepath.Clean(absLower)
	if err := layout.Validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

func (l Layout) Validate() error {
	if l.Root == "" {
		return fmt.Errorf("overlay root cannot be empty")
	}
	root := filepath.Clean(l.Root)
	if root == string(filepath.Separator) {
		return fmt.Errorf("refusing filesystem root as overlay runtime")
	}
	paths := map[string]string{"upper": l.Upper, "work": l.Work, "merged": l.Merged}
	seen := make(map[string]string, len(paths))
	for name, value := range paths {
		if value == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("overlay %s path must be absolute", name)
		}
		clean := filepath.Clean(value)
		if strings.ContainsAny(clean, ",\n\r") {
			return fmt.Errorf("overlay %s path contains an unsupported mount-option character", name)
		}
		rel, err := filepath.Rel(root, clean)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("overlay %s path escapes runtime root: %s", name, clean)
		}
		if previous, exists := seen[clean]; exists {
			return fmt.Errorf("overlay paths %s and %s overlap at %s", previous, name, clean)
		}
		seen[clean] = name
	}
	if l.Lower == "" || !filepath.IsAbs(l.Lower) {
		return fmt.Errorf("overlay lower path must be absolute")
	}
	lower := filepath.Clean(l.Lower)
	if lower == string(filepath.Separator) || strings.ContainsAny(lower, ",\n\r") {
		return fmt.Errorf("overlay lower path is unsafe")
	}
	if isWithin(root, lower) && lower == root {
		return fmt.Errorf("overlay lower path cannot equal runtime root")
	}
	if isWithin(lower, root) {
		return fmt.Errorf("overlay runtime root cannot be inside lower path: %s", lower)
	}
	for name, value := range paths {
		clean := filepath.Clean(value)
		if clean == lower {
			return fmt.Errorf("overlay lower path overlaps %s path", name)
		}
	}
	return nil
}

func isWithin(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (l Layout) Prepare() error {
	if err := l.Validate(); err != nil {
		return err
	}
	for _, path := range []string{l.Root, l.Lower, l.Upper, l.Work, l.Merged} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create overlay path %s: %w", path, err)
		}
	}
	return nil
}

type Runner interface {
	Run(ctx context.Context, command string, args ...string) error
}

// Backend selects the filesystem implementation used for a protected run.
// Kernel uses Linux OverlayFS; Fuse uses fuse-overlayfs for VMs where the
// kernel implementation cannot safely expose copy-up writes to an
// unprivileged agent.
type Backend string

const (
	BackendKernel Backend = "kernel"
	BackendFuse   Backend = "fuse"
)

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

type MountProcess interface {
	Wait() error
	Kill() error
}

type ProcessStarter interface {
	Start(ctx context.Context, command string, args ...string) (MountProcess, error)
}

type ExecProcessStarter struct{}

func (ExecProcessStarter) Start(ctx context.Context, command string, args ...string) (MountProcess, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execMountProcess{cmd: cmd}, nil
}

type execMountProcess struct {
	cmd *exec.Cmd
}

func (p *execMountProcess) Wait() error { return p.cmd.Wait() }
func (p *execMountProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type Manager struct {
	Runner  Runner
	Owner   *Owner
	Backend Backend
	Starter ProcessStarter
}

// Owner identifies the unprivileged agent account that must be able to write
// the temporary upper/work layers after a root parent performs the mount.
// Lowerdir is intentionally never chowned by this manager.
type Owner struct {
	UID int
	GID int
}

func (m Manager) runner() Runner {
	if m.Runner != nil {
		return m.Runner
	}
	return ExecRunner{}
}

func (m Manager) backend() Backend {
	if m.Backend == "" {
		return BackendKernel
	}
	return m.Backend
}

func (m Manager) starter() ProcessStarter {
	if m.Starter != nil {
		return m.Starter
	}
	return ExecProcessStarter{}
}

func (m Manager) Mount(ctx context.Context, l Layout) error {
	if err := l.Prepare(); err != nil {
		return err
	}
	if m.Owner != nil {
		if err := os.Chown(l.Upper, m.Owner.UID, m.Owner.GID); err != nil {
			return fmt.Errorf("chown overlay upper for agent: %w", err)
		}
		if err := os.Chown(l.Work, m.Owner.UID, m.Owner.GID); err != nil {
			return fmt.Errorf("chown overlay work for agent: %w", err)
		}
	}
	switch m.backend() {
	case BackendKernel:
		options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", l.Lower, l.Upper, l.Work)
		return m.runner().Run(ctx, "mount", "-t", "overlay", "overlay", "-o", options, l.Merged)
	case BackendFuse:
		return m.mountFuse(ctx, l)
	default:
		return fmt.Errorf("unsupported overlay backend %q", m.backend())
	}
}

func (m Manager) mountFuse(ctx context.Context, l Layout) error {
	if m.Owner == nil {
		return fmt.Errorf("fuse overlay backend requires an agent owner")
	}
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,uid=%d,gid=%d,allow_other", l.Lower, l.Upper, l.Work, m.Owner.UID, m.Owner.GID)
	process, err := m.starter().Start(ctx, "fuse-overlayfs", "-f", "-o", options, l.Merged)
	if err != nil {
		return fmt.Errorf("start fuse-overlayfs: %w", err)
	}
	if err := waitForMount(ctx, m.runner(), l.Merged, process); err != nil {
		_ = process.Kill()
		return fmt.Errorf("wait for fuse overlay mount: %w", err)
	}
	return nil
}

func waitForMount(ctx context.Context, runner Runner, mountpoint string, process MountProcess) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	processDone := make(chan error, 1)
	go func() { processDone <- process.Wait() }()
	for {
		if err := runner.Run(ctx, "mountpoint", "-q", mountpoint); err == nil {
			return nil
		}
		select {
		case err := <-processDone:
			if err == nil {
				return fmt.Errorf("fuse-overlayfs exited before mount became ready")
			}
			return fmt.Errorf("fuse-overlayfs exited before mount became ready: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("mount did not become ready before timeout")
		case <-ticker.C:
		}
	}
}

func (m Manager) Unmount(ctx context.Context, l Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	command := "umount"
	args := []string{l.Merged}
	if m.backend() == BackendFuse {
		command = "fusermount3"
		args = []string{"-u", l.Merged}
	}
	// FUSE may still be completing the final userspace request when the agent
	// exits. Give the mount a short, bounded settle window; never use a lazy
	// unmount and never discard upper/work while the mount remains busy.
	for attempt := 0; ; attempt++ {
		err := m.runner().Run(ctx, command, args...)
		if err == nil {
			if _, realRunner := m.runner().(ExecRunner); realRunner {
				if err := waitForUnmount(ctx, m.runner(), l.Merged); err != nil {
					return err
				}
			}
			return nil
		}
		if !unmountBusy(err) || attempt >= 20 {
			return err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func waitForUnmount(ctx context.Context, runner Runner, mountpoint string) error {
	for attempt := 0; attempt < 200; attempt++ {
		if err := runner.Run(ctx, "mountpoint", "-q", mountpoint); err != nil {
			return nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("unmount %s did not settle before timeout", mountpoint)
}

func unmountBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "device or resource busy") || strings.Contains(message, "resource busy")
}

// Rollback discards only the validated upper/work directories after unmount.
// It intentionally refuses to proceed if unmount fails.
func (m Manager) Rollback(ctx context.Context, l Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if err := m.Unmount(ctx, l); err != nil {
		// A crashed fuse-overlayfs process can tear down its own mount before
		// the later recovery command runs. Treat the resulting fusermount
		// "already unmounted" error as success; all other unmount failures
		// remain fail-closed so upper/work are never discarded blindly.
		if !(m.backend() == BackendFuse && alreadyUnmounted(err)) {
			return fmt.Errorf("rollback unmount: %w", err)
		}
	}
	for _, path := range []string{l.Upper, l.Work} {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("rollback remove %s: %w", path, err)
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("rollback recreate %s: %w", path, err)
		}
	}
	return nil
}

func alreadyUnmounted(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid argument") ||
		strings.Contains(message, "not found in /etc/mtab") ||
		strings.Contains(message, "not mounted")
}
