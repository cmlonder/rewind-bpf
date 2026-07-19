// Package cgroup owns the per-run cgroup-v2 process boundary.
//
// A cgroup is deliberately kept separate from Landlock and eBPF: it provides
// lifecycle identity and cleanup for the complete process tree, while those
// packages provide filesystem policy and telemetry.
package cgroup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultRoot = "/sys/fs/cgroup"

type Scope struct {
	path string
}

// FromPath reopens a persisted run scope for cleanup after the original
// process has exited. The path is accepted only under the cgroup-v2 rewind
// namespace, preventing a run record from targeting an arbitrary cgroup.
func FromPath(path string) (Scope, error) {
	clean := filepath.Clean(path)
	root := filepath.Clean(defaultRoot)
	prefix := filepath.Join(root, "rewind") + string(filepath.Separator)
	if !strings.HasPrefix(clean, prefix) {
		return Scope{}, fmt.Errorf("open cgroup: unsafe persisted path %s", path)
	}
	if _, err := os.Stat(clean); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Scope{}, nil
		}
		return Scope{}, fmt.Errorf("open cgroup %s: %w", clean, err)
	}
	return Scope{path: clean}, nil
}

// New creates a dedicated child cgroup under the host's cgroup-v2 mount.
func New(runID string) (Scope, error) {
	return NewAt(defaultRoot, runID)
}

// NewAt is injectable for unit tests and does not touch the host cgroup tree
// unless the caller explicitly supplies a real cgroup mount.
func NewAt(root, runID string) (Scope, error) {
	if strings.TrimSpace(root) == "" {
		return Scope{}, fmt.Errorf("create cgroup: root cannot be empty")
	}
	if strings.TrimSpace(runID) == "" {
		return Scope{}, fmt.Errorf("create cgroup: run id cannot be empty")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Scope{}, fmt.Errorf("create cgroup: resolve root: %w", err)
	}
	controllers, err := os.ReadFile(filepath.Join(root, "cgroup.controllers"))
	if err != nil {
		return Scope{}, fmt.Errorf("create cgroup: cgroup-v2 unavailable at %s: %w", root, err)
	}
	if strings.TrimSpace(string(controllers)) == "" {
		return Scope{}, fmt.Errorf("create cgroup: cgroup-v2 controllers are unavailable at %s", root)
	}
	parent := filepath.Join(root, "rewind")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Scope{}, fmt.Errorf("create cgroup: create parent: %w", err)
	}
	name := "run-" + sanitize(runID)
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		return Scope{}, fmt.Errorf("create cgroup %s: %w", path, err)
	}
	return Scope{path: path}, nil
}

func sanitize(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func (s Scope) Path() string { return s.path }

// AddPID moves the helper into the scope. All descendants inherit the
// cgroup unless an explicitly privileged process moves itself elsewhere.
func (s Scope) AddPID(pid uint32) error {
	if s.path == "" || pid == 0 {
		return fmt.Errorf("add pid to cgroup: scope and pid are required")
	}
	if err := os.WriteFile(filepath.Join(s.path, "cgroup.procs"), []byte(strconv.FormatUint(uint64(pid), 10)), 0o600); err != nil {
		return fmt.Errorf("add pid %d to cgroup %s: %w", pid, s.path, err)
	}
	return nil
}

// Terminate kills every remaining member. cgroup.kill is atomic on supported
// cgroup-v2 kernels; the PID fallback is retained for older VM kernels.
func (s Scope) Terminate() error {
	if s.path == "" {
		return nil
	}
	killPath := filepath.Join(s.path, "cgroup.kill")
	if err := os.WriteFile(killPath, []byte("1\n"), 0o600); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.EINVAL) {
		return fmt.Errorf("terminate cgroup %s: %w", s.path, err)
	}
	data, err := os.ReadFile(filepath.Join(s.path, "cgroup.procs"))
	if err != nil {
		return fmt.Errorf("terminate cgroup %s: read members: %w", s.path, err)
	}
	var errs []error
	for _, line := range strings.Fields(string(data)) {
		pid, parseErr := strconv.Atoi(line)
		if parseErr != nil || pid < 1 {
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			errs = append(errs, fmt.Errorf("kill pid %d: %w", pid, err))
		}
	}
	return errors.Join(errs...)
}

// Close removes an empty per-run cgroup. It refuses to remove a scope that
// still contains a process, making leaked descendants visible to callers.
func (s Scope) Close() error {
	if s.path == "" {
		return nil
	}
	deadline := time.Now().Add(1 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(s.path, "cgroup.procs"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("close cgroup %s: read members: %w", s.path, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("close cgroup %s: %w", s.path, err)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("close cgroup %s: processes remain", s.path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
