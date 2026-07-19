// Package netpolicy compiles the network section into an auditable decision
// plan. It deliberately does not open sockets or inject credentials.
package netpolicy

import (
	"fmt"
	"strings"

	"github.com/rewindbpf/rewind/internal/policy"
)

type Decision string

const (
	Allow Decision = "allow"
	Audit Decision = "audit"
	Deny  Decision = "deny"
)

type Plan struct {
	Mode           policy.Mode
	AllowDomains   []string
	CredentialRefs []policy.CredentialRef
}

func Compile(value policy.NetworkPolicy) (Plan, error) {
	if err := value.Validate(); err != nil {
		return Plan{}, fmt.Errorf("compile network policy: %w", err)
	}
	plan := Plan{Mode: value.Mode}
	plan.AllowDomains = append([]string(nil), value.AllowDomains...)
	plan.CredentialRefs = append([]policy.CredentialRef(nil), value.CredentialRefs...)
	return plan, nil
}

// Explain is a pure preview operation. Enforcing network egress requires a
// platform backend; callers must refuse to advertise enforcement when one is
// unavailable.
func (p Plan) Explain(host string) Decision {
	if p.Mode == policy.ModeOff {
		return Allow
	}
	if p.Mode == policy.ModeAudit {
		return Audit
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, allowed := range p.AllowDomains {
		allowed = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(allowed), "."))
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return Allow
		}
	}
	return Deny
}
