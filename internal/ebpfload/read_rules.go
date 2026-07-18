package ebpfload

import (
	"fmt"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/rewindbpf/rewind/internal/policycompile"
)

const readRuleMapName = "rewind_read_rules"

// ReadRuleKey mirrors the fixed-size key used by the BPF read-policy map.
// Keeping the conversion here makes the userspace/kernel ABI explicit and
// unit-testable without loading a BPF object.
type ReadRuleKey struct {
	Path [256]byte
}

func makeReadRuleKey(path string) (ReadRuleKey, error) {
	if strings.IndexByte(path, 0) >= 0 {
		return ReadRuleKey{}, fmt.Errorf("read rule path contains NUL")
	}
	if len([]byte(path)) > 255 {
		return ReadRuleKey{}, fmt.Errorf("read rule path exceeds 255 bytes: %s", path)
	}
	var key ReadRuleKey
	copy(key.Path[:], path)
	return key, nil
}

func decisionCode(decision string) (uint32, error) {
	switch decision {
	case "audit":
		return 1, nil
	case "deny":
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported kernel read decision %q", decision)
	}
}

// InstallReadRules replaces the caller-owned read-rule map contents with the
// compiled exact-path rules. The map is updated only after all keys and values
// have been validated, so malformed policy data cannot be partially installed.
func InstallReadRules(m *ebpf.Map, rules policycompile.ReadRules) error {
	if m == nil {
		return fmt.Errorf("install read rules: map is nil")
	}
	if rules.Mode == "off" {
		return nil
	}
	for _, rule := range rules.Rules {
		key, err := makeReadRuleKey(rule.Path)
		if err != nil {
			return fmt.Errorf("install read rules: %w", err)
		}
		value, err := decisionCode(rule.Decision)
		if err != nil {
			return fmt.Errorf("install read rules for %s: %w", rule.Path, err)
		}
		if err := m.Update(&key, value, ebpf.UpdateAny); err != nil {
			return fmt.Errorf("install read rule %s: %w", rule.Path, err)
		}
	}
	return nil
}
