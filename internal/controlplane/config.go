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

	"github.com/rewindbpf/rewind/internal/agent"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policybundle"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type PolicyPackage struct {
	Name         string               `json:"name"`
	Version      string               `json:"version"`
	Description  string               `json:"description,omitempty"`
	Policy       policy.Policy        `json:"policy"`
	Signed       bool                 `json:"signed"`
	SignerKeyID  string               `json:"signer_key_id,omitempty"`
	SignedBundle *policybundle.Signed `json:"signed_bundle,omitempty"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type Workspace struct {
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	Policy    string     `json:"policy"`
	Adapter   agent.Kind `json:"adapter,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
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

// UpdatePolicy replaces an existing package in place. Package identity is the
// name+version reference used by workspace assignments; changing identity is a
// new package operation, which keeps active assignments deterministic.
func (s *Store) UpdatePolicy(value PolicyPackage) error {
	if err := validatePolicyPackage(value); err != nil {
		return err
	}
	return s.mutate(func(snapshot *Snapshot) error {
		for i := range snapshot.Policies {
			if snapshot.Policies[i].Name == value.Name && snapshot.Policies[i].Version == value.Version {
				if snapshot.Policies[i].Signed {
					return fmt.Errorf("signed policy package %s@%s is immutable; publish a new version", value.Name, value.Version)
				}
				value.UpdatedAt = time.Now().UTC()
				value.Signed = snapshot.Policies[i].Signed
				value.SignerKeyID = snapshot.Policies[i].SignerKeyID
				value.SignedBundle = snapshot.Policies[i].SignedBundle
				snapshot.Policies[i] = value
				return nil
			}
		}
		return fmt.Errorf("policy package %s@%s does not exist", value.Name, value.Version)
	})
}

// CreateSignedPolicy verifies the self-contained Ed25519 envelope before it is
// admitted to the local policy catalog. Trust distribution is deliberately
// separate: callers that need an allow-list can verify the envelope against
// trusted keys before calling this method.
func (s *Store) CreateSignedPolicy(signed policybundle.Signed) error {
	bundle, err := policybundle.Verify(signed)
	if err != nil {
		return err
	}
	return s.CreatePolicy(PolicyPackage{
		Name:         bundle.Name,
		Version:      bundle.Version,
		Description:  bundle.Description,
		Policy:       bundle.Policy,
		Signed:       true,
		SignerKeyID:  signed.KeyID,
		SignedBundle: &signed,
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
	for _, value := range snapshot.Policies {
		if !value.Signed || value.SignedBundle == nil {
			continue
		}
		bundle, err := policybundle.Verify(*value.SignedBundle)
		if err != nil {
			return Snapshot{}, fmt.Errorf("verify stored signed policy %s@%s: %w", value.Name, value.Version, err)
		}
		if bundle.Name != value.Name || bundle.Version != value.Version || value.SignerKeyID != value.SignedBundle.KeyID {
			return Snapshot{}, fmt.Errorf("stored signed policy %s@%s metadata mismatch", value.Name, value.Version)
		}
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
	if _, err := agent.Resolve(string(value.Adapter)); err != nil {
		return err
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
