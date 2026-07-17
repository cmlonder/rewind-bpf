package policy

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeOff     Mode = "off"
	ModeAudit   Mode = "audit"
	ModeEnforce Mode = "enforce"
)

type Policy struct {
	Read    ReadPolicy    `yaml:"read" json:"read"`
	Write   WritePolicy   `yaml:"write" json:"write"`
	Network NetworkPolicy `yaml:"network" json:"network"`
}

type ReadPolicy struct {
	Mode  Mode     `yaml:"mode" json:"mode"`
	Deny  []string `yaml:"deny" json:"deny"`
	Allow []string `yaml:"allow" json:"allow"`
}

type WritePolicy struct {
	Mode  string `yaml:"mode" json:"mode"`
	Scope string `yaml:"scope" json:"scope"`
}

type NetworkPolicy struct {
	Mode Mode `yaml:"mode" json:"mode"`
}

func Load(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy %s: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (Policy, error) {
	var value Policy
	if err := yaml.Unmarshal(data, &value); err != nil {
		return Policy{}, fmt.Errorf("parse policy: %w", err)
	}
	if err := value.Validate(); err != nil {
		return Policy{}, err
	}
	return value, nil
}

func (p Policy) Validate() error {
	if err := validateMode("read.mode", p.Read.Mode, true); err != nil {
		return err
	}
	if err := validateMode("network.mode", p.Network.Mode, true); err != nil {
		return err
	}
	if p.Write.Mode != "" && p.Write.Mode != "rollback" {
		return fmt.Errorf("write.mode must be rollback when set, got %q", p.Write.Mode)
	}
	if p.Write.Scope != "" && p.Write.Scope != "workspace" && p.Write.Scope != "system" {
		return fmt.Errorf("write.scope must be workspace or system when set, got %q", p.Write.Scope)
	}
	for _, pattern := range append(append([]string{}, p.Read.Deny...), p.Read.Allow...) {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("read patterns cannot be empty")
		}
	}
	return nil
}

func validateMode(name string, value Mode, emptyAllowed bool) error {
	if value == "" && emptyAllowed {
		return nil
	}
	switch value {
	case ModeOff, ModeAudit, ModeEnforce:
		return nil
	default:
		return fmt.Errorf("%s must be off, audit, or enforce; got %q", name, value)
	}
}

// Match implements slash-aware globbing with ** matching zero or more path
// segments. A pattern without a leading slash can match an absolute path at
// any depth, which makes patterns such as **/.env useful to users.
func Match(pattern, candidate string) bool {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	candidate = strings.TrimSpace(strings.ReplaceAll(candidate, "\\", "/"))
	if pattern == "" || candidate == "" {
		return false
	}
	if !strings.HasPrefix(pattern, "/") {
		candidate = strings.TrimPrefix(candidate, "/")
	}
	patternSegments := split(pattern)
	candidateSegments := split(candidate)
	return matchSegments(patternSegments, candidateSegments)
}

func (p ReadPolicy) Decision(candidate string) string {
	for _, pattern := range p.Allow {
		if Match(pattern, candidate) {
			return "allow"
		}
	}
	for _, pattern := range p.Deny {
		if Match(pattern, candidate) {
			switch p.Mode {
			case ModeAudit:
				return "audit"
			case ModeEnforce:
				return "deny"
			default:
				return "allow"
			}
		}
	}
	return "allow"
}

func split(value string) []string {
	value = strings.Trim(value, "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func matchSegments(pattern, candidate []string) bool {
	if len(pattern) == 0 {
		return len(candidate) == 0
	}
	if pattern[0] == "**" {
		return matchSegments(pattern[1:], candidate) || (len(candidate) > 0 && matchSegments(pattern, candidate[1:]))
	}
	if len(candidate) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], candidate[0])
	return err == nil && matched && matchSegments(pattern[1:], candidate[1:])
}
