package overlay

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func (l Layout) Validate() error {
	if l.Root == "" {
		return fmt.Errorf("overlay root cannot be empty")
	}
	root := filepath.Clean(l.Root)
	if root == string(filepath.Separator) {
		return fmt.Errorf("refusing filesystem root as overlay runtime")
	}
	paths := map[string]string{
		"lower":  l.Lower,
		"upper":  l.Upper,
		"work":   l.Work,
		"merged": l.Merged,
	}
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
	return nil
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

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

type Manager struct {
	Runner Runner
}

func (m Manager) runner() Runner {
	if m.Runner != nil {
		return m.Runner
	}
	return ExecRunner{}
}

func (m Manager) Mount(ctx context.Context, l Layout) error {
	if err := l.Prepare(); err != nil {
		return err
	}
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", l.Lower, l.Upper, l.Work)
	return m.runner().Run(ctx, "mount", "-t", "overlay", "overlay", "-o", options, l.Merged)
}

func (m Manager) Unmount(ctx context.Context, l Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	return m.runner().Run(ctx, "umount", l.Merged)
}

// Rollback discards only the validated upper/work directories after unmount.
// It intentionally refuses to proceed if unmount fails.
func (m Manager) Rollback(ctx context.Context, l Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if err := m.Unmount(ctx, l); err != nil {
		return fmt.Errorf("rollback unmount: %w", err)
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
