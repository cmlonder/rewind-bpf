// Package controlplane stores the small, durable configuration surface used by
// the local supervisor. It never stores workspace contents or raw credentials.
package controlplane

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rewindbpf/rewind/internal/policy"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type PolicyPackage struct {
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description,omitempty"`
	Policy      policy.Policy `json:"policy"`
	Signed      bool          `json:"signed"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type Workspace struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Policy    string    `json:"policy"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Snapshot struct {
	Revision   uint64          `json:"revision"`
	Policies   []PolicyPackage `json:"policies"`
	Workspaces []Workspace     `json:"workspaces"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func Open(path string) *Store { return &Store{path: strings.TrimSpace(path)} }

func (s *Store) Enabled() bool { return s != nil && s.path != "" }

func (s *Store) Snapshot() (Snapshot, error) {
	if !s.Enabled() {
		return Snapshot{}, fmt.Errorf("control-plane config store is disabled")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

func (s *Store) CreatePolicy(value PolicyPackage) error {
	if err := validatePolicyPackage(value); err != nil {
		return err
	}
	return s.mutate(func(snapshot *Snapshot) error {
		for _, existing := range snapshot.Policies {
			if existing.Name == value.Name && existing.Version == value.Version {
				return fmt.Errorf("policy package %s@%s already exists", value.Name, value.Version)
			}
		}
		value.UpdatedAt = time.Now().UTC()
		snapshot.Policies = append(snapshot.Policies, value)
		return nil
	})
}

func (s *Store) AssignWorkspace(value Workspace) error {
	if err := validateWorkspace(value); err != nil {
		return err
	}
	return s.mutate(func(snapshot *Snapshot) error {
		if value.Policy != "none" && !hasPolicy(snapshot.Policies, value.Policy) {
			return fmt.Errorf("workspace policy %q is not installed", value.Policy)
		}
		value.UpdatedAt = time.Now().UTC()
		for i := range snapshot.Workspaces {
			if snapshot.Workspaces[i].Name == value.Name {
				snapshot.Workspaces[i] = value
				return nil
			}
		}
		snapshot.Workspaces = append(snapshot.Workspaces, value)
		return nil
	})
}

func (s *Store) mutate(fn func(*Snapshot) error) error {
	if !s.Enabled() {
		return fmt.Errorf("control-plane config store is disabled")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, err := s.readLocked()
	if err != nil {
		return err
	}
	if err := fn(&snapshot); err != nil {
		return err
	}
	snapshot.Revision++
	return s.writeLocked(snapshot)
}

func (s *Store) readLocked() (Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return Snapshot{Policies: []PolicyPackage{}, Workspaces: []Workspace{}}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("read control-plane config: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode control-plane config: %w", err)
	}
	if snapshot.Policies == nil {
		snapshot.Policies = []PolicyPackage{}
	}
	if snapshot.Workspaces == nil {
		snapshot.Workspaces = []Workspace{}
	}
	return snapshot, nil
}

func (s *Store) writeLocked(snapshot Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create control-plane config directory: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode control-plane config: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write control-plane config: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit control-plane config: %w", err)
	}
	return nil
}

func validatePolicyPackage(value PolicyPackage) error {
	if !identifierPattern.MatchString(value.Name) {
		return fmt.Errorf("policy package name must match %s", identifierPattern.String())
	}
	if !versionPattern.MatchString(value.Version) {
		return fmt.Errorf("policy package version must be semantic x.y.z")
	}
	if len(value.Description) > 240 {
		return fmt.Errorf("policy package description is too long")
	}
	return value.Policy.Validate()
}

func validateWorkspace(value Workspace) error {
	if !identifierPattern.MatchString(value.Name) {
		return fmt.Errorf("workspace name must match %s", identifierPattern.String())
	}
	if !filepath.IsAbs(value.Path) || filepath.Clean(value.Path) == string(filepath.Separator) {
		return fmt.Errorf("workspace path must be an absolute non-root path")
	}
	if strings.TrimSpace(value.Policy) == "" {
		return fmt.Errorf("workspace policy is required; use none to leave unassigned")
	}
	return nil
}

func hasPolicy(policies []PolicyPackage, reference string) bool {
	for _, value := range policies {
		if value.Name+"@"+value.Version == reference {
			return true
		}
	}
	return false
}
